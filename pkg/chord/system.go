package chord

import (
	"syscall"
	"fmt"
	"net"
	"os"
)



func Send(conn *net.UnixConn, data []byte, f *os.File) error {
	return send(conn, data, int(f.Fd()))
}

func send(conn *net.UnixConn, data []byte, fds ...int) error {
	_, _, err := conn.WriteMsgUnix(data, syscall.UnixRights(fds...), nil)
	return err
}

func Receive(conn *net.UnixConn) ([]byte, *os.File, error) {
	for {
		data, fds, err := receive(conn)
		if err != nil {
			return nil, nil, fmt.Errorf("receive: %v", err)
		}
		if len(fds) == 0 {
			continue
		}
		if len(fds) > 1 {
			for _, fd := range fds {
				syscall.Close(fd)
			}
		}
		return data, os.NewFile(uintptr(fds[0]), ""), nil
	}
	panic("impossibru")
	return nil, nil, nil
}

func receive(conn *net.UnixConn) ([]byte, []int, error) {
	buf := make([]byte, 4096)
	oob := make([]byte, 4096)
	bufn, oobn, _, _, err := conn.ReadMsgUnix(buf, oob)
	if err != nil {
		return nil, nil, err
	}
	return buf[:bufn], extractFds(oob[:oobn]), nil
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

func socketpair() ([2]int, error) {
	return syscall.Socketpair(syscall.AF_LOCAL, syscall.SOCK_STREAM, 0)
}

var SocketPair = socketpair


func fdconn(fd int) (*net.UnixConn, error) {
	f := os.NewFile(uintptr(fd), fmt.Sprintf("%d", fd))
	conn, err := net.FileConn(f)
	if err != nil {
		return nil, err
	}
	uconn, ok := conn.(*net.UnixConn)
	if !ok {
		return nil, fmt.Errorf("%d: not a unix connection", fd)
	}
	return uconn, nil
}

var FdConn = fdconn
