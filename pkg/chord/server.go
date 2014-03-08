package chord

import (
	"syscall"
	"os"
	"net"
	"fmt"
)

func NewServerSession(conn *net.UnixConn) *ServerSession {
	return &ServerSession{conn}
}

type ServerSession struct {
	*net.UnixConn
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
