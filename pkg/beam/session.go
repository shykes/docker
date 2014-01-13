package beam

import (
	"bytes"
	"fmt"
	"github.com/dotcloud/docker/pkg/beam/data"
	"io"
	"net"
	"os"
)

type Session struct {
	conn      *Conn
	connError error
	chReceive chan *Stream
	chSend    chan *Stream
	streams   map[uint32]*Stream
	isServer  bool
	routes    []*Route
}

func New(conn *net.UnixConn, server bool) *Session {
	session := &Session{
		conn:      &Conn{conn},
		streams:   make(map[uint32]*Stream),
		isServer:  server,
		chReceive: make(chan *Stream, 4096),
		chSend:    make(chan *Stream, 4096),
	}
	go session.sendLoop()
	go session.receiveLoop()
	return session
}

func (session *Session) sendStream(s *Stream) error {
	defer s.remote.Close()
	data := s.infoMsg().Bytes()
	fds := []int{int(s.remote.Fd())}
	if err := session.conn.Send(data, fds); err != nil {
		return fmt.Errorf("send: %s", err)
	}
	return nil
}

func (session *Session) sendLoop() error {
	var id int
	if session.isServer {
		id = 2
	} else {
		id = 1
	}
	for s := range session.chSend {
		s.id = uint32(id)
		if _, exists := session.streams[s.id]; exists {
			panic("outgoing id conflict")
		}
		session.streams[s.id] = s
		// Send on the wire
		err := session.sendStream(s)
		s.chErr <- err
		if err != nil {
			return err
		}
		if id+2 > 0xffffffff {
			return fmt.Errorf("can't allocate new id: uint32 overflow")
		}
		id += 2
	}
	return nil
}

func (session *Session) receiveLoop() (e error) {
	defer func() {
		if e == nil {
			session.connError = io.EOF
		} else {
			session.connError = e
		}
		close(session.chReceive)
	}()
	var id int
	if session.isServer {
		id = 1
	} else {
		id = 2
	}
	for {
		buf, fds, err := session.conn.Receive()
		if err != nil {
			return fmt.Errorf("receive: %s", err)
		}
		if len(fds) >= 1 {
			// We received at least one fd.
			// Use the first for data.
			// If the second exists, use it for metadata.
			// Ignore any other FDs.
			fd := fds[0]
			var metaFd int
			if len(fds) >= 2 {
				metaFd = fds[1]
			} else {
				metaFd = -1
			}
			info := make(data.Msg)
			if _, err := info.ReadFrom(bytes.NewReader(buf)); err != nil {
				// Invalid stream information. Skip this message.
				fmt.Printf("Skipping invalid stream information (%d bytes)\n", len(buf))
				continue
			}
			// FInd and validate the parent stream, if specified.
			var parent *Stream
			if info.Exists("parent-id") {
				if parentId64, err := info.GetInt("parent-id"); err != nil {
					fmt.Printf("Rejecting invalid stream parent-id: %s\n", err)
					continue
				} else {
					if p, exists := session.streams[uint32(parentId64)]; !exists {
						fmt.Printf("Rejecting stream with non-existent parent-id %d\n", parentId64)
						continue
					} else {
						parent = p
					}
				}
			}
			s := session.New(parent)
			// Extract an initial header, if any.
			if info.Exists("header") {
				if metadata, err := info.GetMsg("header"); err != nil {
					fmt.Printf("Rejecting stream with invalid header (%d bytes)\n", len(info.Get("header")))
					continue
				} else {
					s.Metadata = metadata
				}
			}
			// Validate the stream id.
			if id64, err := info.GetUint("id"); err != nil {
				fmt.Printf("Skipping invalid stream id: %s\n", err)
				continue
			} else {
				if int(id64) != id {
					// Skip incorrect id.
					// FIXME: send a protocol error
					continue
				}
				s.id = uint32(id64)
			}
			s.local = os.NewFile(uintptr(fd), fmt.Sprintf("%d", fd))
			s.metaLocal = os.NewFile(uintptr(metaFd), fmt.Sprintf("%d", fd))
			if _, exists := session.streams[s.id]; exists {
				// Skip stream with already existing id
				// (this shouldn't happen because we increment the id every time anyway)
				continue
			}
			session.streams[s.id] = s
			fmt.Printf("--> %s\n", s)
			if s.parent == nil {
				session.chReceive <-s
			} else {
				s.parent.chReceive <-s
			}
		}
		id += 2
	}
	panic("Unreachable")
	return nil
}

func (session *Session) Close() error {
	close(session.chSend)
	// FIXME: flush outgoing messages
	return session.conn.Close()
}

func (session *Session) Receive() (stream *Stream, e error) {
	if session.connError != nil {
		return nil, session.connError
	}
	return <-session.chReceive, session.connError
}

func (session *Session) New(parent *Stream) *Stream {
	return &Stream{
		parent:   parent,
		session:  session,
		Metadata: make(data.Msg),
		chReceive: make(chan *Stream, 4096),
	}
}
