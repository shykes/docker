package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"syscall"
)

const FILENAME string = ".example0.sock"

func main() {
	conn, server, err := connect(FILENAME)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
	defer conn.Close()
	fmt.Printf("server=%v\n", server)
	if server {
		fmt.Printf("Reading from socket...\n")
		_, fds, err := Receive(conn)
		if err != nil {
			fmt.Fprintf(os.Stderr, "receive: %s\n", err)
			os.Exit(1)

		}
		if len(fds) < 1 {
			fmt.Fprintf(os.Stderr, "receive: no fd received\n")
			os.Exit(1)
		}
		if _, err := io.Copy(os.Stdout, os.NewFile(uintptr(fds[0]), "peer")); err != nil {
			fmt.Fprintf(os.Stderr, "copy: %s\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Printf("Reading from %s...\n", os.Args[1])
		f, err := os.Open(os.Args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "open: %s\n", err)
			os.Exit(1)
		}
		defer f.Close()
		if err := Send(conn, []byte{}, []int{int(f.Fd())}); err != nil {
			fmt.Fprintf(os.Stderr, "send: %s\n", err)
			os.Exit(1)
		}
	}
}

func connect(filename string) (conn *net.UnixConn, server bool, err error) {
	addr, err := net.ResolveUnixAddr("unixgram", filename)
	if err != nil {
		return nil, false, fmt.Errorf("resolveaddr: %s", err)
	}
	conn, err = net.DialUnix("unixgram", nil, addr)
	if err != nil {
		os.Remove(filename)
		conn, err = net.ListenUnixgram("unixgram", addr)
		if err != nil {
			return nil, false, fmt.Errorf("listen: %s", err)
		}
		return conn, true, nil
	}
	return conn, false, nil
}

func Receive(conn *net.UnixConn) (data []byte, fds []int, err error) {
	var oob []byte = make([]byte, 4096)
	data = make([]byte, 4096)
	_, oobn, _, _, err := conn.ReadMsgUnix(data, oob)
	if err != nil {
		return nil, nil, fmt.Errorf("readmsg: %s", err)
	}
	fds = extractFds(oob[:oobn])
	return
}

func Send(conn *net.UnixConn, data []byte, fds []int) error {
	_, _, err := conn.WriteMsgUnix(data, syscall.UnixRights(fds...), nil)
	return err
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
