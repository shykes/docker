package unix

import (
	"bytes"
	"fmt"
	"io"
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
	fd int
	data io.ReadWriteCloser
	metaFd int
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

func (t *Transport) ReceiveStream() (stream *Stream, e error) {
	defer func() {
		fmt.Printf("received stream: id=%d parent=%v err=%v\n", stream.Id(), stream.Parent(), e)
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
			fmt.Printf("stream info = %v\n", info)
			// FInd and validate the parent stream, if specified.
			var parent *Stream
			if info.Exists("parent-id") {
				fmt.Printf("parent-id exists, parsing\n")
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
			s := t.newStream(fd, metaFd)
			s.id = id
			s.parent = parent
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

func (t *Transport) newStream(fd, metaFd int) *Stream {
	return &Stream{
		fd: fd,
		data: os.NewFile(uintptr(fd), fmt.Sprintf("%d", fd)),
		metaFd: metaFd,
		transport: t,
	}
}

func (t *Transport) SendStream(parent *Stream) (stream *Stream, err error) {
	defer func() {
		fmt.Printf("sent stream: id=%d parent=%v err=%v\n", stream.Id(), stream.Parent(), err)
	}()
	// Our transport must guarantee both 1) ordered delivery of octet streams
	// and 2) protected message boundaries.
	// We have the following options:
	//
	// Option 1: use SOCK_SEQPACKET which offers both guarantees natively.
	// This is the best option, but is not available on all systems.
	//
	// Option 2: use SOCK_DGRAM which guarantees message boundaries but not ordered
	// delivery. In practice most implementations don't re-order datagrams, so with
	// some fact-checking it might be ok to use this on modern systems.
	//
	// Option 3: use SOCK_STREAM which guarantees ordered delivery but not message
	// boundaries. This requires layering a custom framing protocol over the socket.
	// This will be required anyway for TCP-based transports.
	//
	// See unix(7) and unixpair(2)
	pair, err := syscall.Socketpair(syscall.AF_LOCAL, syscall.SOCK_STREAM, 0)
	if err != nil {
		return nil, fmt.Errorf("socketpair: %s", err)
	}
	// We send one fd (arbitrarily: 0) and keep the other (1).
	defer func() {
		// Always close the remote fd.
		syscall.Close(pair[0])
		// Only close the local fd if there's an error.
		if err != nil {
			syscall.Close(pair[1])
		}
	}()
	s := t.newStream(pair[1], -1)
	s.parent = parent
	// Register the new stream, setting id to 0 to auto-assign
	if err := t.Set(0, s, false); err != nil {
		return nil, err
	}
	// Generate info message
	info := make(data.Msg)
	info.SetInt("id", int64(s.id))
	if p := s.Parent(); p != nil {
		info.SetInt("parent-id", int64(p.Id()))
	}
	if err := Send(t.conn, info.Bytes(), []int{pair[0]}); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Stream) Hijack() (fd int, err error) {
	return s.fd, nil
}

func (s *Stream) Read(d []byte) (int, error) {
	return s.data.Read(d)
}

func (s *Stream) Write(d []byte) (int, error) {
	return s.data.Write(d)
}

func (s *Stream) Close() error {
	return syscall.Close(s.fd)
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
