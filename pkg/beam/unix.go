package beam

import (
	"bufio"
	"container/list"
	"fmt"
	"net"
	"os"
	"syscall"
)

type BeamConn struct {
	*net.UnixConn
	fds list.List
}

func NewFromUnix(conn *net.UnixConn) *BeamConn {
	return &BeamConn{UnixConn: conn}
}

// Framing:

// In order to handle framing in Send/Recieve, as these give frame
// boundaries we use a very simple 4 bytes header. It is a big endiand
// uint32 where the high bit is set if the message includes a file
// descriptor. The rest of the uint32 is the length of the next frame.
// We need the bit in order to be able to assign recieved fds to
// the right message, as multiple messages may be coalesced into
// a single recieve operation.

func debugCheckpoint(msg string, args ...interface{}) {
	if os.Getenv("DEBUG") == "" {
		return
	}
	os.Stdout.Sync()
	tty, _ := os.OpenFile("/dev/tty", os.O_RDWR, 0700)
	fmt.Fprintf(tty, msg, args...)
	bufio.NewScanner(tty).Scan()
	tty.Close()
}

// Send sends a new message on conn with data and f as payload and
// attachment, respectively.
// On success, f is closed
func (conn *BeamConn) Send(data []byte, f *os.File) error {
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
	if err := conn.sendUnix(data, fds...); err != nil {
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
func (conn *BeamConn) Receive() (rdata []byte, rf *os.File, rerr error) {
	defer func() {
		var fd int = -1
		if rf != nil {
			fd = int(rf.Fd())
		}
		debugCheckpoint("===DEBUG=== Receive() -> '%s'[%d]. Hit enter to continue.\n", rdata, fd)
	}()

	// Read header
	header := make([]byte, 4)
	nRead := uint32(0)

	for nRead < 4 {
		n, err := conn.receiveUnix(header[nRead:])
		if err != nil {
			return nil, nil, err
		}
		nRead = nRead + uint32(n)
	}

	length, hasFd := parseHeader(header)

	if hasFd {
		front := conn.fds.Front()
		if front == nil {
			return nil, nil, fmt.Errorf("No expected file descriptor in message")
		}

		rf = front.Value.(*os.File)
	}

	rdata = make([]byte, length)

	nRead = 0
	for nRead < length {
		n, err := conn.receiveUnix(rdata[nRead:])
		if err != nil {
			return nil, nil, err
		}
		nRead = nRead + uint32(n)
	}

	return
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
func (conn *BeamConn) SendPipe(data []byte) (endpoint *net.UnixConn, err error) {
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
	if err := conn.Send(data, remote); err != nil {
		return nil, err
	}
	return endpoint, nil
}

func (conn *BeamConn) SendBeam(data []byte) (*BeamConn, error) {
	endpoint, err := conn.SendPipe(data)
	if err != nil {
		return nil, err
	}
	return NewFromUnix(endpoint), nil
}

func (conn *BeamConn) receiveUnix(buf []byte) (int, error) {
	oob := make([]byte, syscall.CmsgSpace(4))
	bufn, oobn, _, _, err := conn.ReadMsgUnix(buf, oob)
	if err != nil {
		return 0, err
	}
	fds := extractFds(oob[:oobn], 1)
	if len(fds) >= 1 {
		f := os.NewFile(uintptr(fds[0]), "")
		conn.fds.PushBack(f)
	}

	return bufn, nil
}

func makeHeader(data []byte, fds []int) ([]byte, error) {
	header := make([]byte, 4)

	length := uint32(len(data))

	if length > 0x7fffffff {
		return nil, fmt.Errorf("Data to large")
	}

	if len(fds) != 0 {
		length = length | 0x80000000
	}
	header[0] = byte((length >> 24) & 0xff)
	header[1] = byte((length >> 16) & 0xff)
	header[2] = byte((length >> 8) & 0xff)
	header[3] = byte((length >> 0) & 0xff)

	return header, nil
}

func parseHeader(header []byte) (uint32, bool) {
	length := uint32(header[0])<<24 | uint32(header[1])<<16 | uint32(header[2])<<8 | uint32(header[3])
	hasFd := length&0x80000000 != 0
	length = length & ^uint32(0x80000000)

	return length, hasFd
}

func (conn *BeamConn) sendUnix(data []byte, fds ...int) error {
	header, err := makeHeader(data, fds)
	if err != nil {
		return err
	}

	// There is a bug in conn.WriteMsgUnix where it doesn't correctly return
	// the number of bytes writte (http://code.google.com/p/go/issues/detail?id=7645)
	// So, we can't rely on the return value from it. However, we must use it to
	// send the fds. In order to handle this we only write one byte using WriteMsgUnix
	// (when we have to), as that can only ever block or fully suceed. We then write
	// the rest with conn.Write()
	// The reader side should not rely on this though, as hopefully this gets fixed
	// in go later.
	written := 0
	if len(fds) != 0 {
		oob := syscall.UnixRights(fds...)
		wrote, _, err := conn.WriteMsgUnix(header[0:1], oob, nil)
		if err != nil {
			return err
		}
		written = written + wrote
	}

	for written < len(header) {
		wrote, err := conn.Write(header[written:])
		if err != nil {
			return err
		}
		written = written + wrote
	}

	written = 0
	for written < len(data) {
		wrote, err := conn.Write(data[written:])
		if err != nil {
			return err
		}
		written = written + wrote
	}

	return nil
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
	defer debugCheckpoint("===DEBUG=== USocketPair() returned. Hit enter to confirm ")
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

func BeamPair() (*BeamConn, *BeamConn, error) {
	a, b, err := USocketPair()
	if err != nil {
		return nil, nil, err
	}

	return NewFromUnix(a), NewFromUnix(b), nil
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
