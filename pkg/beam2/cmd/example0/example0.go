package main

import (
	"net"
	"fmt"
	"os"
	"io"
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
		if _, err := io.Copy(os.Stdout, conn); err != nil {
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
		if _, err := io.Copy(conn, f); err != nil {
			fmt.Fprintf(os.Stderr, "copy: %s\n", err)
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
