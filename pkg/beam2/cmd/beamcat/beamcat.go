package main

import (
	"io"
	"os"
	"bufio"
	"os/exec"
	"fmt"
	"net"
	"sync"
	"strings"
	"net/http"
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
		st, err := t.SendStream(nil)
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
		if st.Parent() != nil {
			go io.Copy(os.Stdout, st)
			continue
		}
		scanner := bufio.NewScanner(st)
		scanner.Scan()
		if err := scanner.Err(); err != nil {
			log.Fatal("read from peer: %s", err)
		}
		fmt.Printf("---> %s\n", scanner.Text())
		wg.Add(1)
		go func() {
			words := strings.Split(scanner.Text(), " ")
			stdout, err := t.SendStream(st)
			if err != nil {
				return
			}
			stderr, err := t.SendStream(st)
			if err != nil {
				return
			}
			if words[0] == "download" {
				if len(words) < 2 {
					fmt.Fprintf(stderr, "Error: please specify a url\n")
					fmt.Fprintf(st, "status=1\n")
				} else {
					fmt.Fprintf(stderr, "Downloading from %s\n", words[1])
					resp, err := http.Get(words[1])
					if err != nil {
						fmt.Fprintf(stderr, "get: %s\n", err)
						fmt.Fprintf(st, "status=2\n")
					} else {
						fmt.Fprintf(stderr, "%s\n", resp.Status)
						io.Copy(stdout, resp.Body)
					}
				}
			} else {
				cmd := exec.Command("sh", "-c", scanner.Text())
				cmd.Stdout = stdout
				cmd.Stderr = stderr
				if err := cmd.Run(); err != nil {
					fmt.Fprintf(st, "error: %s\n", err)
				}
				fmt.Fprintf(st, "status=%s\n", cmd.ProcessState)
			}
			st.Close()
			stdout.Close()
			stderr.Close()
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
