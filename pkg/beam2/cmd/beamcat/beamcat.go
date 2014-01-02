package main

import (
	"io"
	"os"
	"bufio"
	"os/exec"
	"fmt"
	"net"
	"sync"
	"log"
	"github.com/dotcloud/docker/pkg/beam2/unix"
)

func main() {
	sock, server, err := connectStream(".beam.sock")
	if err != nil {
		log.Fatal(err)
	}
	transport := unix.New(sock, server)
	defer transport.Close()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		handleUserInput(os.Stdin, transport)
		wg.Done()
	}()
	go func() {
		handleRequests(transport, os.Stdout)
		wg.Done()
	}()
	wg.Wait()
}

func handleUserInput(src io.Reader, t *unix.Transport) {
	defer fmt.Printf("handleUserInput done\n")
	var wg sync.WaitGroup
	defer wg.Wait()
	input := bufio.NewScanner(src)
	for input.Scan() {
		if err := input.Err(); err != nil {
			log.Fatal("stdin: %s", err)
		}
		st, err := t.SendStream()
		if err != nil {
			log.Fatalf("sendstream: %s", err)
		}
		if _, err := fmt.Fprintf(st, "%s\n", input.Text()); err != nil {
			log.Fatalf("write: %s", err)
		}
		wg.Add(1)
		go func() {
			_, err := io.Copy(os.Stdout, st)
			if err != nil {
				log.Printf("Error reading from stream: %s", err)
			}
			fmt.Printf("[%d] Closed\n", st.Id())
			wg.Done()
		}()
	}
}

func handleRequests(t *unix.Transport, dst io.Writer) {
	defer fmt.Printf("handleRequests done\n")
	var wg sync.WaitGroup
	defer wg.Wait()
	for {
		st, err := t.ReceiveStream()
		if err != nil {
			log.Fatalf("receivestream: %s", err)
		}
		scanner := bufio.NewScanner(st)
		scanner.Scan()
		if err := scanner.Err(); err != nil {
			log.Fatal("read from peer: %s", err)
		}
		fmt.Printf("---> %s\n", scanner.Text())
		wg.Add(1)
		go func() {
			cmd := exec.Command("sh", "-c", scanner.Text())
			cmd.Stdout = st
			cmd.Stderr = st
			if err := cmd.Run(); err != nil {
				fmt.Fprintf(st, "error: %s\n", err)
			}
			st.Close()
		}()
	}
}

func connectStream(filename string) (conn *net.UnixConn, server bool, err error) {
	addr, err := net.ResolveUnixAddr("unix", filename)
	if err != nil {
		return nil, false, fmt.Errorf("resolveaddr: %s", err)
	}
	conn, err = net.DialUnix("unix", nil, addr)
	if err != nil {
		os.Remove(filename)
		l, err := net.ListenUnix("unix", addr)
		if err != nil {
			return nil, false, fmt.Errorf("listen: %s", err)
		}
		conn, err := l.AcceptUnix()
		if err != nil {
			return nil, false, fmt.Errorf("accept: %s", err)
		}
		return conn, true, nil
	}
	return conn, false, nil
}

func connectGram(filename string) (conn *net.UnixConn, server bool, err error) {
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
