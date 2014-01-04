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
		scanner := bufio.NewScanner(st)
		scanner.Scan()
		if err := scanner.Err(); err != nil {
			log.Fatal("read from peer: %s", err)
		}
		fmt.Printf("---> %s\n", scanner.Text())
		wg.Add(1)
		go func() {
			words := strings.Split(strings.Trim(scanner.Text(), " \t"), " ")
			job := &Job{
				Stream: st,
				Name: words[0],
				Args: words[1:],
			}
			job.Printf("job-name=%s\n", job.Name)
			job.Printf("job-args=%s\n", strings.Join(job.Args, "\x00"))
			job.Stdout = job.New()
			if err := job.Stdout.Send(); err != nil {
				log.Fatalf("send stdout: %s", err)
			}
			job.Stderr = job.New()
			if err := job.Stderr.Send(); err != nil {
				log.Fatalf("send stderr: %s", err)
			}
			switch job.Name {
				case "download": jobDownload(job)
				case "listen":   jobListen(job)
				case "exec":     jobExec(job)
				default: {
					job.Stderr.Printf("No such command: %s\n", job.Name)
					job.Printf("status=2\n")
				}
			}
			job.Stdout.Close()
			job.Stderr.Close()
			job.Close()
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

func jobListen(job *Job) {
	if len(job.Args) < 2 {
		job.Stderr.Printf("Usage: %s PROTO ADDRESS\n", job.Name)
		job.Printf("status=1\n")
		return
	}
	proto := job.Args[0]
	addr := job.Args[1]
	job.Stderr.Printf("Listening on %s/%s", proto, addr)
	l, err := net.Listen(proto, addr)
	if err != nil {
		job.Stderr.Printf("listen: %s\n", err)
		job.Printf("status=1\n")
		return
	}
	for {
		conn, err := l.Accept()
		if err != nil {
			job.Stderr.Printf("accept: %s\n", err)
			job.Printf("status=1\n")
			return
		}
		job.Stderr.Printf("New connection from %s\n", conn.RemoteAddr())
		st := job.New()
		if err := st.Send(); err != nil {
			job.Stderr.Printf("send: %s\n", err)
			job.Printf("status=1\n")
			return
		}
		go func() {
			Splice(st, conn)
			conn.Close()
			st.Close()
		}()
	}
}

func Splice(a, b io.ReadWriter) (err error) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		_, e := io.Copy(a, b)
		if e != nil && err == nil {
			err = e
		}
		wg.Done()
	}()
	go func() {
		_, e := io.Copy(b, a)
		if e != nil && err == nil {
			err = e
		}
		wg.Done()
	}()
	wg.Wait()
	return
}

func jobExec(job *Job) {
	cmd := exec.Command("sh", "-c", strings.Join(job.Args, " "))
	cmd.Stdout = job.Stdout
	cmd.Stderr = job.Stderr
	if err := cmd.Run(); err != nil {
		job.Stderr.Printf("error: %s\n", err)
		job.Printf("status=127\n")
		return
	}
	job.Printf("status=%s\n", cmd.ProcessState)
}

type Job struct {
	*unix.Stream
	Name string
	Args []string
	Stdout *unix.Stream
	Stderr *unix.Stream
}


func jobDownload(job *Job) {
	if len(job.Args) < 1 {
		job.Stderr.Printf("Usage: %s URL\n", job.Name)
		job.Printf("status=1\n")
		return
	}
	url := job.Args[0]
	job.Stderr.Printf("Downloading from %s\n", url)
	resp, err := http.Get(url)
	if err != nil {
		job.Stderr.Printf("GET %s: %s\n", url, err)
		job.Printf("status=1\n")
		return
	}
	job.Stderr.Printf("%s\n", resp.Status)
	io.Copy(job.Stdout, resp.Body)
	job.Printf("status=0\n")
}
