package chord

import (
	"fmt"
	"net"
	"os"
	"syscall"
	"io"
)

const (
	DefaultServerFd = 3
	DefaultClientFd = 4
)

func NewClient(conn *net.UnixConn) (*Client, error) {
	if conn == nil {
		if c, err := fdconn(DefaultClientFd); err != nil {
			return nil, fmt.Errorf("fd %v is not a valid unix socket", DefaultClientFd)
		} else {
			conn = c
		}
	}
	return &Client{conn}, nil
}

type Client struct {
	*net.UnixConn
}

// FIXME: this is needed to "cast" a client as server
// (for nested worker scenarios)
// The correct solution is probably to merge Server and Client
// (while avoiding recursive brain meltdown).
func (c *Client) Conn() *net.UnixConn {
	return c.UnixConn
}

func (c *Client) Open(name string) (e *Endpoint, err error) {
	pair, err := socketpair()
	if err != nil {
		return nil, err
	}
	local := pair[0]
	remote := pair[1]
	defer func() {
		if err != nil {
			syscall.Close(local)
		}
		syscall.Close(remote)
	}()
	if err := send(c.UnixConn, []byte(name), remote); err != nil {
		return nil, err
	}
	localConn, err := fdconn(local)
	if err != nil {
		return nil, err
	}
	return &Endpoint{name: name, conn: localConn}, nil
}


type Endpoint struct {
	conn *net.UnixConn
	name string
}

func (e *Endpoint) Accept() (net.Conn, error) {
	_, f, err := e.ReceiveFile()
	if err != nil {
		return nil, err
	}
	return net.FileConn(f)
}

func (e *Endpoint) Close() error {
	return e.conn.Close()
}

func (e *Endpoint) Addr() net.Addr {
	return chordAddr(e.name)
}

type chordAddr string

func (addr chordAddr) Network() string {
	return "chord"
}

func (addr chordAddr) String() string {
	return string(addr)
}

func (e *Endpoint) Receive() (name string, conn io.ReadWriteCloser, err error) {
	for {
		data, fds, err := receive(e.conn)
		if err != nil {
			return "", nil, fmt.Errorf("receive: %v", err)
		}
		if len(fds) != 1 {
			// Skip message with too little or too many attachments
			continue
		}
		if len(data) > 0 && string(data) != e.name {
			// Skip message not matching service name
			continue
		}
		// We received a valid message
		name = string(data)
		conn = os.NewFile(uintptr(fds[0]), fmt.Sprintf("%s[%d]", name, fds[0]))
		break
	}
	return name, conn, nil
}

func (e *Endpoint) ReceiveFile() (string, *os.File, error) {
	name, conn, err := e.Receive()
	f, _ := conn.(*os.File)
	return name, f, err
}


// FIXME: typed helpers
// Connect

