package beam

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"io"
	"bufio"
	"github.com/dotcloud/docker/pkg/beam/data"
)

type Session struct {
	conn *Conn
	idsIn *IdCounter
	idsOut *IdCounter
	streams map[uint32]*Stream
}

type Stream struct {
	id uint32
	parent *Stream
	Metadata data.Msg
	local *os.File
	remote *os.File
	metaLocal *os.File
	metaRemote *os.File
	session *Session
}

func New(conn *net.UnixConn, server bool) *Session {
	return &Session{
		conn: &Conn{conn},
		idsOut: &IdCounter{ odd: !server},
		idsIn:  &IdCounter{ odd: server},
		streams: make(map[uint32]*Stream),
	}
}

func (session *Session) Close() error {
	return session.conn.Close()
}

func (session *Session) Set(id uint32, stream *Stream, inbound bool) error {
	var ids *IdCounter
	if inbound {
		ids = session.idsIn
	} else {
		ids = session.idsOut
	}
	actualId, err := ids.Register(id)
	if err != nil {
		return err
	}
	if _, exists := session.streams[actualId]; exists {
		return fmt.Errorf("stream already exists: %d", id)
	}
	stream.id = actualId
	session.streams[actualId] = stream
	return nil
}

func (session *Session) Get(id uint32) *Stream {
	if s, exists := session.streams[id]; exists {
		return s
	}
	return nil
}

func (session *Session) Receive() (stream *Stream, e error) {
	defer func() {
		// fmt.Printf("received stream: id=%d parent=%v err=%v\n", stream.Id(), stream.Parent(), e)
	}()
	for {
		buf, fds, err := session.conn.Receive()
		if err != nil {
			return nil, fmt.Errorf("receive: %s", err)
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
					parent = session.Get(uint32(parentId64))
					if parent == nil {
						fmt.Printf("Rejecting stream with non-existent parent-id %d\n", parentId64)
						continue
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
			var id uint32
			if id64, err := info.GetUint("id"); err != nil {
				fmt.Printf("Skipping invalid stream id: %s\n", err)
				continue
			} else {
				id = uint32(id64)
			}
			s.id = id
			s.local = os.NewFile(uintptr(fd), fmt.Sprintf("%d", fd))
			s.metaLocal = os.NewFile(uintptr(metaFd), fmt.Sprintf("%d", fd))
			if err := session.Set(id, s, true); err != nil {
				fmt.Printf("Rejecting invalid stream id: %s\n", err)
				continue
			}
			return s, nil
		}
	}
	return nil, fmt.Errorf("unexpectedly reached end of read loop")
}

func (session *Session) New(parent *Stream) *Stream {
	return &Stream{
		parent: parent,
		session: session,
		Metadata: make(data.Msg),
	}
}

func (s *Stream) Send(presend ...func(id int)) error {
	if s.id != 0 {
		return fmt.Errorf("stream already registered as id=%d", s.id)
	}
	// If no file has been set with SetFile, setup a socketpair.
	if s.remote == nil {
		local, remote, err := Socketpair()
		if err != nil {
			return fmt.Errorf("socketpair: %s", err)
		}
		s.SetFile(remote)
		s.local = local
	}
	// Register the new stream, setting id to 0 to auto-assign
	if err := s.session.Set(0, s, false); err != nil {
		return err
	}
	// Now we have the final ID
	for _, fn := range presend {
		fn(s.Id())
	}
	// FIXME: serialize sending to avoid race conditions in ID numbering
	if err := s.session.conn.Send(s.infoMsg().Bytes(), []int{int(s.remote.Fd())}); err != nil {
		return fmt.Errorf("send: %s", err)
	}
	s.remote.Close()
	return nil
}

func (s *Stream) New() *Stream {
	return s.session.New(s)
}

func (s *Stream) infoMsg() data.Msg {
	info := make(data.Msg)
	info.SetInt("id", int64(s.id))
	if p := s.Parent(); p != nil {
		info.SetInt("parent-id", int64(p.Id()))
	}
	// Send initial metadata, if any, as a nested "header" field
	if len(s.Metadata) > 0 {
		info.Set("header", s.Metadata.String())
	}
	return info
}

func (s *Stream) SetFile(f *os.File) {
	s.remote = f
	s.local = nil
}

func (s *Stream) GetFile() (f *os.File, err error) {
	if s.local != nil {
		return f, nil
	}
	return nil, fmt.Errorf("local endpoint not available")
}

func (s *Stream) Read(d []byte) (int, error) {
	if s.local == nil {
		return 0, fmt.Errorf("read: local endpoint not available")
	}
	return s.local.Read(d)
}

func (s *Stream) Write(d []byte) (int, error) {
	if s.local == nil {
		return 0, fmt.Errorf("write: local endpoint not available")
	}
	return s.local.Write(d)
}

func (s *Stream) Printf(format string, args ...interface{}) (int, error) {
	return fmt.Fprintf(s, format, args...)
}

func (s *Stream) TailTo(dst io.Writer, prefix string) error {
	scanner := bufio.NewScanner(s)
	for scanner.Scan() {
		if line := scanner.Text(); line != "" {
			if _, err := fmt.Fprintf(dst, "%s%s\n", prefix, line); err != nil {
				return err
			}
		}
		if err := scanner.Err(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Stream) Close() error {
	if s.local == nil {
		return fmt.Errorf("close: local endpoint not available")
	}
	s.local.Sync()
	return s.local.Close()
}

func (s *Stream) Id() int {
	return int(s.id)
}

func (s *Stream) Parent() *Stream {
	return s.parent
}
