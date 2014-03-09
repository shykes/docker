package chord

import (
	"syscall"
	"os"
	"net"
	"fmt"
)

func NewServerSession(conn *net.UnixConn) (*ServerSession, error) {
	if conn == nil {
		if c, err := fdconn(DefaultServerFd); err != nil {
			return nil, fmt.Errorf("fd %v is not a valid unix socket", DefaultClientFd)
		} else {
			conn = c
		}
	}
	return &ServerSession{conn}, nil
}

type ServerSession struct {
	*net.UnixConn
}

func (srv *ServerSession) Receive() (string, *net.UnixConn, error) {
	for {
		data, f, err := Receive(srv.UnixConn)
		if err != nil {
			return "", nil, err
		}
		conn, err := fdconn(int(f.Fd()))
		if err != nil {
			f.Close()
			// A valid client request must attach a unix socket, so that
			// the server can send endpoints back.
			continue
		}
		return string(data), conn, nil
	}
	panic("impossibru")
	return "", nil, nil
}

func (srv *ServerSession) Serve(handler func([]byte, *net.UnixConn)) error {
	for {
		data, fds, err := receive(srv.UnixConn)
		if err != nil {
			return fmt.Errorf("receive: %v", err)
		}
		if len(fds) == 0 {
			continue
		}
		if len(fds) > 1 {
			for _, fd := range fds {
				syscall.Close(fd)
			}
		}
		go func(data []byte, fd int) {
			defer syscall.Close(fd)
			conn, err := fdconn(fd)
			if err != nil {
				return
			}
			handler(data, conn)
		}(data, fds[0])
	}
	return nil
}

type Handler func(string, chan *os.File)

func (srv *ServerSession) ServeSimple(h Handler) error {
	return srv.Serve(func(data []byte, client *net.UnixConn) {
		name := string(data)
		endpoints := make(chan *os.File)
		go func() {
			h(string(data), endpoints)
			close(endpoints)
		}()
		for ep := range endpoints {
			if err := send(client, []byte(name), int(ep.Fd())); err != nil {
				return
			}
		}
	})
}
