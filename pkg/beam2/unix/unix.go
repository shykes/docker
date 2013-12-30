package unix

import (
	"fmt"
	"net"
	"syscall"
	"github.com/dotcloud/docker/pkg/beam2"
	"github.com/dotcloud/docker/pkg/beam2/data"
)

type Transport struct {
	conn *net.UnixConn
	nextId uint64
}

type Stream struct {
	fd int
	metaFd int
	flag int
	transport *Transport
}

func New(conn *net.UnixConn) *Transport {
	return &Transport{
		conn: conn,
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

func (t *Transport) ReceiveStream(flag int) (stream beam2.Stream, err error) {
	for {
		var (
			buf []byte
			oob []byte
		)
		_, oobn, _, _, err := t.conn.ReadMsgUnix(buf, oob)
		if err != nil {
			return nil, err
		}
		/*
		var msg data.StructuredMessage
		if _, err := msg.ReadFrom(bytes.NewReader(buf[:n])); err != nil {
			// Invalid metadata. Skip this message.
			fmt.Printf("Skipping invalid message (%d bytes)\n", n)
			continue
		}
		if v, err := msg.GetInt("version"); err != nil || v != 1 {
			fmt.Printf("Skipping message with invalid version '%s'", msg.Get("version"))
			continue
		}
		if id, err := msg.GetInt("id"); err != nil {
			fmt.Printf("Skipping message with invalid sequence id: %s", err)
			continue
		else if id != t.nextId {
			fmt.Printf("Skipping message with out of sequence id: %d", id)
			continue
		}
		*/
		if fds := extractFds(oob[:oobn]); len(fds) >= 1 {
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
			return &Stream{
				data: os.NewFile(fd, fmt.Sprintf("%d", fd))
				fd: fd,
				metaFd: metaFd,
				flag: flag,
				transport: t,
			}, nil
		}
	}
	return nil, fmt.Errorf("unexpectedly reached end of read loop")
}

func (t *Transport) SendStream(flag int) (stream beam2.Stream, err error) {
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
	pair, err := syscall.Socketpair(syscall.SOCK_SEQPACKET, syscall.AF_UNIX, 0)
	if err != nil {
		return nil, err
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
	rights := syscall.UnixRights(pair[0])
	if _, _, err := t.conn.WriteMsgUnix([]byte{}, rights, nil); err != nil {
		return nil, err
	}
	// FIXME: enforce flags directly on the fd with fcntl.
	return &Stream{
		fd: pair[1],
		flag: flag,
		transport: t,
	}, nil
}

func (s *Stream) Receive() (beam2.Frame, error) {
	var (
		f []byte
		oob []byte
	)
	_, _, _, _, err := syscall.Recvmsg(s.fd, f, oob, 0)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (s *Stream) Hijack() (fd int, err error) {
	return s.fd, nil
}

func (s *Stream) Send(f beam2.Frame) error {
	return syscall.Sendmsg(s.fd, f, nil, nil, 0)
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
