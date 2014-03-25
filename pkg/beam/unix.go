package beam

import (
	"fmt"
	"net"
	"os"
	"syscall"
	"bufio"
)

func debugCheckpoint(msg string, args ...interface{}) {
	if os.Getenv("DEBUG") == "" {
		return
	}
	os.Stdout.Sync()
	tty,_ := os.OpenFile("/dev/tty", os.O_RDWR, 0700)
	fmt.Fprintf(tty, msg, args...)
	bufio.NewScanner(tty).Scan()
	tty.Close()
}

// Send sends a new message on conn with data and f as payload and
// attachment, respectively.
// On success, f is closed
func Send(conn *net.UnixConn, data []byte, f *os.File) error {
	{
		var fd int = -1
		if f != nil {
			fd = int(f.Fd())
		}
		debugCheckpoint("===DEBUG=== about to send '%s'[%d]. Hit enter to confirm: ", data, fd)
	}
	var fds []int
	if f != nil {
		fds = append(fds, int(f.Fd()))
	}
	if err := sendUnix(conn, data, fds...); err != nil {
		return err
	}

	if f != nil {
		f.Close()
	}
	return nil
}

// Receive waits for a new message on conn, and receives its payload
// and attachment, or an error if any.
//
// If more than 1 file descriptor is sent in the message, they are all
// closed except for the first, which is the attachment.
// It is legal for a message to have no attachment or an empty payload.
func Receive(conn *net.UnixConn) (rdata []byte, rf *os.File, rerr error) {
	defer func() {
		var fd int = -1
		if rf != nil {
			fd = int(rf.Fd())
		}
		debugCheckpoint("===DEBUG=== Receive() -> '%s'[%d]. Hit enter to continue.\n", rdata, fd)
	}()
	for {
		data, fds, err := receiveUnix(conn, 1)
		if err != nil {
			return nil, nil, err
		}
		var f *os.File
		if len(fds) >= 1 {
			f = os.NewFile(uintptr(fds[0]), "")
		}
		return data, f, nil
	}
	panic("impossibru")
	return nil, nil, nil
}

// SendPipe creates a new unix socket pair, sends one end as the attachment
// to a beam message with the payload `data`, and returns the other end.
//
// This is a common pattern to open a new service endpoint.
// For example, a service wishing to advertise its presence to clients might
// open an endpoint with:
//
//  endpoint, _ := SendPipe(conn, []byte("sql"))
//  defer endpoint.Close()
//  for {
//  	conn, _ := endpoint.Receive()
//	go func() {
//		Handle(conn)
//		conn.Close()
//	}()
//  }
//
// Note that beam does not distinguish between clients and servers in the logical
// sense: any program wishing to establishing a communication with another program
// may use SendPipe() to create an endpoint.
// For example, here is how an application might use it to connect to a database client.
//
//  endpoint, _ := SendPipe(conn, []byte("userdb"))
//  defer endpoint.Close()
//  conn, _ := endpoint.Receive()
//  defer conn.Close()
//  db := NewDBClient(conn)
//
// In this example note that we only need the first connection out of the endpoint,
// but we could open new ones to retry after a broken connection.
// Note that, because the underlying service transport is abstracted away, this
// allows for arbitrarily complex service discovery and retry logic to take place,
// without complicating application code.
//
func SendPipe(conn *net.UnixConn, data []byte) (endpoint *net.UnixConn, err error) {
	debugCheckpoint("===DEBUG=== SendPipe('%s'). Hit enter to confirm: ", data)
	local, remote, err := SocketPair()
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			local.Close()
			remote.Close()
		}
	}()
	endpoint, err = FdConn(local)
	if err != nil {
		return nil, err
	}
	local.Close()
	if err := Send(conn, data, remote); err != nil {
		return nil, err
	}
	return endpoint, nil
}

func receiveUnix(conn *net.UnixConn, maxFds int) ([]byte, []int, error) {
	buf := make([]byte, 4096)
	oob := make([]byte, syscall.CmsgSpace(maxFds*4))
	bufn, oobn, _, _, err := conn.ReadMsgUnix(buf, oob)
	if err != nil {
		return nil, nil, err
	}
	return buf[:bufn], extractFds(oob[:oobn], maxFds), nil
}

func sendUnix(conn *net.UnixConn, data []byte, fds ...int) error {
	_, _, err := conn.WriteMsgUnix(data, syscall.UnixRights(fds...), nil)
	return err
}

func extractFds(oob []byte, maxFds int) (fds []int) {
	// Grab forklock to make sure no forks accidentally inherit the new
	// fds before they are made CLOEXEC
	// There is a slight race condition between ReadMsgUnix returns and
	// when we grap the lock, so this is not perfect. Unfortunately
	// There is no way to pass MSG_CMSG_CLOEXEC to recvmsg() nor any
	// way to implement non-blocking i/o in go, so this is hard to fix.
	syscall.ForkLock.Lock()
	defer syscall.ForkLock.Unlock()
	scms, err := syscall.ParseSocketControlMessage(oob)
	if err != nil {
		return
	}
	for _, scm := range scms {
		gotFds, err := syscall.ParseUnixRights(&scm)
		if err != nil {
			continue
		}
		for i, fd := range gotFds {
			if i >= maxFds {
				syscall.Close(fd)
			} else {
				syscall.CloseOnExec(fd)
				fds = append(fds, fd)
			}
		}
	}
	return
}

func socketpair() ([2]int, error) {
	return syscall.Socketpair(syscall.AF_LOCAL, syscall.SOCK_STREAM|syscall.FD_CLOEXEC, 0)
}

// SocketPair is a convenience wrapper around the socketpair(2) syscall.
// It returns a unix socket of type SOCK_STREAM in the form of 2 file descriptors
// not bound to the underlying filesystem.
// Messages sent on one end are received on the other, and vice-versa.
// It is the caller's responsibility to close both ends.
func SocketPair() (a *os.File, b *os.File, err error) {
	defer func() {
		var (
			fdA int = -1
			fdB int = -1
		)
		if a != nil {
			fdA = int(a.Fd())
		}
		if b != nil {
			fdB = int(b.Fd())
		}
		debugCheckpoint("===DEBUG=== SocketPair() = [%d-%d]. Hit enter to confirm: ", fdA, fdB)
	}()
	pair, err := socketpair()
	if err != nil {
		return nil, nil, err
	}
	return os.NewFile(uintptr(pair[0]), ""), os.NewFile(uintptr(pair[1]), ""), nil
}

func USocketPair() (*net.UnixConn, *net.UnixConn, error) {
	debugCheckpoint("===DEBUG=== USocketPair(). Hit enter to confirm: ")
	defer debugCheckpoint ("===DEBUG=== USocketPair() returned. Hit enter to confirm ")
	a, b, err := SocketPair()
	if err != nil {
		return nil, nil, err
	}
	defer a.Close()
	defer b.Close()
	uA, err := FdConn(a)
	if err != nil {
		return nil, nil, err
	}

	uB, err := FdConn(b)
	if err != nil {
		uA.Close()
		return nil, nil, err
	}

	return uA, uB, nil
}

// FdConn wraps a file descriptor in a standard *net.UnixConn object, or
// returns an error if the file descriptor does not point to a unix socket.
// This creates a duplicate file descriptor. It's the caller's responsibility
// to close both.
func FdConn(f *os.File) (*net.UnixConn, error) {
	{
		debugCheckpoint("===DEBUG=== FdConn([%d]) = (unknown fd). Hit enter to confirm: ", f.Fd())
	}
	conn, err := net.FileConn(f)
	if err != nil {
		return nil, err
	}
	uconn, ok := conn.(*net.UnixConn)
	if !ok {
		conn.Close()
		return nil, fmt.Errorf("%d: not a unix connection", f.Fd())
	}
	return uconn, nil
}
