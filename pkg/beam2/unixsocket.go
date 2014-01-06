package unix

import (
	"net"
	"syscall"
	"fmt"
)

type Conn struct {
	*net.UnixConn
}

func (conn *Conn) Send(data []byte, fds[]int) (err error) {
	_, _, err = conn.WriteMsgUnix(data, syscall.UnixRights(fds...), nil)
	return err
}

func (conn *Conn) Receive() (data []byte, fds []int, err error) {
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

