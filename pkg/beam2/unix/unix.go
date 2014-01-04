package unix

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"os"
	"syscall"
	"github.com/dotcloud/docker/pkg/beam2/data"
)

type Transport struct {
	conn *net.UnixConn
	idsIn *IdCounter
	idsOut *IdCounter
	streams map[uint32]*Stream
}

type Stream struct {
	id uint32
	parent *Stream
	header http.Header
	local *os.File
	remote *os.File
	metaLocal *os.File
	metaRemote *os.File
	transport *Transport
}

func New(conn *net.UnixConn, server bool) *Transport {
	return &Transport{
		conn: conn,
		idsOut: &IdCounter{ odd: !server},
		idsIn:  &IdCounter{ odd: server},
		streams: make(map[uint32]*Stream),
	}
}

func extractFds(oob []byte) (fds []int) {
	scms, err := syscall.ParseSocketControlMessage(oob)
	if err != nil {
		return
	}
	for _, scm := range scms {
		gotFds, err := syscall.ParseUnixRights(&scm)
		if err != nil {
			continue
		}
		fds = append(fds, gotFds...)
	}
	return
}

func (t *Transport) Close() error {
	return t.conn.Close()
}

func (t *Transport) Set(id uint32, stream *Stream, inbound bool) error {
	var ids *IdCounter
	if inbound {
		ids = t.idsIn
	} else {
		ids = t.idsOut
	}
	actualId, err := ids.Register(id)
	if err != nil {
		return err
	}
	if _, exists := t.streams[actualId]; exists {
		return fmt.Errorf("stream already exists: %d", id)
	}
	stream.id = actualId
	t.streams[actualId] = stream
	return nil
}

func (t *Transport) Get(id uint32) *Stream {
	if s, exists := t.streams[id]; exists {
		return s
	}
	return nil
}

func Receive(conn *net.UnixConn) (data []byte, fds []int, err error) {
	buf := make([]byte, 4096)
	oob := make([]byte, 4096)
	bufn, oobn, _, _, err := conn.ReadMsgUnix(buf, oob)
	if err != nil {
		return nil, nil, fmt.Errorf("readmsg: %s", err)
	}
	fds = extractFds(oob[:oobn])
	data = buf[:bufn]
	return
}

func Send(conn *net.UnixConn, data []byte, fds[]int) (err error) {
	_, _, err = conn.WriteMsgUnix(data, syscall.UnixRights(fds...), nil)
	return err
}

func (t *Transport) Receive() (stream *Stream, e error) {
	defer func() {
		// fmt.Printf("received stream: id=%d parent=%v err=%v\n", stream.Id(), stream.Parent(), e)
	}()
	for {
		buf, fds, err := Receive(t.conn)
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
					parent = t.Get(uint32(parentId64))
					if parent == nil {
						fmt.Printf("Rejecting stream with non-existent parent-id %d\n", parentId64)
						continue
					}
				}
			}
			// Extract an initial header, if any.
			var header http.Header
			if info.Exists("header") {
				if headerData, err := info.GetMsg("heder"); err != nil {
					fmt.Printf("Rejecting stream with invalid header (%d bytes)\n", len(info.Get("header")))
					continue
				} else {
					header = headerData.ToHTTPHeader()
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
			s := t.New(parent)
			s.id = id
			s.local = os.NewFile(uintptr(fd), fmt.Sprintf("%d", fd))
			s.metaLocal = os.NewFile(uintptr(metaFd), fmt.Sprintf("%d", fd))
			s.header = header
			if err := t.Set(id, s, true); err != nil {
				fmt.Printf("Rejecting invalid stream id: %s\n", err)
				continue
			}
			return s, nil
		}
	}
	return nil, fmt.Errorf("unexpectedly reached end of read loop")
}

func (t *Transport) New(parent *Stream) *Stream {
	return &Stream{
		parent: parent,
		transport: t,
	}
}

func (s *Stream) Send() error {
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
	if err := s.transport.Set(0, s, false); err != nil {
		return err
	}
	if err := Send(s.transport.conn, s.infoMsg().Bytes(), []int{int(s.remote.Fd())}); err != nil {
		return fmt.Errorf("send: %s", err)
	}
	s.remote.Close()
	return nil
}

func (s *Stream) infoMsg() data.Msg {
	info := make(data.Msg)
	info.SetInt("id", int64(s.id))
	if p := s.Parent(); p != nil {
		info.SetInt("parent-id", int64(p.Id()))
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

func (s *Stream) Close() error {
	if s.local == nil {
		return fmt.Errorf("close: local endpoint not available")
	}
	s.local.Sync()
	return s.local.Close()
}

func (s *Stream) Metadata() data.StructuredStream {
	// FIXME
	return nil
}

func (s *Stream) Id() int {
	return int(s.id)
}

func (s *Stream) Parent() *Stream {
	return s.parent
}
