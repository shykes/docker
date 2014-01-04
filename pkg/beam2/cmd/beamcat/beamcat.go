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
		job := t.New(nil)
		if err := job.Send(); err != nil {
			log.Fatalf("send: %s", err)
		}
		if _, err := job.Printf("%s\n", input.Text()); err != nil {
			log.Fatalf("write: %s", err)
		}
		wg.Add(1)
		go func(cmdline string) {
			err := copyLines(os.Stdout, job, fmt.Sprintf("[%d] [%s] ", job.Id(), cmdline))
			if err != nil {
				log.Printf("Error reading from stream: %s", err)
			}
			fmt.Printf("[%d] [%s] Closed\n", job.Id(), cmdline)
			wg.Done()
		}(input.Text())
	}
}

func copyLines(dst io.Writer, src io.Reader, prefix string) error {
	scanner := bufio.NewScanner(src)
	for scanner.Scan() {
		if line := scanner.Text(); line != "" {
			if _, err := fmt.Fprintf(dst, "%s%s\n", prefix, line); err != nil {
				return err
			}
		}
		if err := scanner.Err(); err != nil {
			return err
		}
	}
	return nil
}

func handleRequests(t *unix.Transport, dst io.Writer) {
	defer fmt.Printf("handleRequests done\n")
	var wg sync.WaitGroup
	defer wg.Wait()
	for {
		st, err := t.Receive()
		if err != nil {
			log.Fatalf("receive: %s", err)
		}
		if st.Parent() != nil {
			go copyLines(os.Stdout, st, fmt.Sprintf("[%d/%d] ", st.Parent().Id(), st.Id()))
			continue
		}
		job := st
		scanner := bufio.NewScanner(job)
		scanner.Scan()
		if err := scanner.Err(); err != nil {
			log.Fatal("read from peer: %s", err)
		}
		fmt.Printf("---> %s\n", scanner.Text())
		wg.Add(1)
		go func() {
			words := strings.Split(scanner.Text(), " ")
			stdout := t.New(job)
			if err := stdout.Send(); err != nil {
				log.Fatalf("send stdout: %s", err)
			}
			stderr := t.New(job)
			if err := stderr.Send(); err != nil {
				log.Fatalf("send stderr: %s", err)
			}
			if words[0] == "download" {
				if len(words) < 2 {
					stderr.Printf("Error: please specify a url\n")
					job.Printf("status=1\n")
				} else {
					stderr.Printf("Downloading from %s\n", words[1])
					resp, err := http.Get(words[1])
					if err != nil {
						stderr.Printf("get: %s\n", err)
						job.Printf("status=2\n")
					} else {
						fmt.Fprintf(stderr, "%s\n", resp.Status)
						io.Copy(stdout, resp.Body)
						job.Printf("status=0\n")
					}
				}
			} else {
				cmd := exec.Command("sh", "-c", scanner.Text())
				cmd.Stdout = stdout
				cmd.Stderr = stderr
				if err := cmd.Run(); err != nil {
					stderr.Printf("error: %s\n", err)
					job.Printf("status=127\n")
				} else {
					fmt.Fprintf(st, "status=%s\n", cmd.ProcessState)
				}
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
