package main

import (
	"bufio"
	"fmt"
	"github.com/dotcloud/docker/pkg/beam"
	"github.com/dotcloud/docker/pkg/beam/jobs"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
)

func main() {
	sock, server, err := connectStream(".beam.sock")
	if err != nil {
		log.Fatal(err)
	}
	session := beam.New(sock, server)
	defer session.Close()
	srv := jobs.NewServer()
	srv.Register("download", jobDownload)
	srv.Register("listen", jobListen)
	srv.Register("exec", jobExec)
	srv.Register("echo", jobEcho)
	srv.Register("cat", jobCat)
	srv.Bind(session)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		if err := session.Run(); err != nil {
			log.Fatal(err)
		}
		wg.Done()
	}()
	go func() {
		handleUserInput(os.Stdin, session)
		wg.Done()
	}()
	wg.Wait()
}

func handleUserInput(src io.Reader, session *beam.Session) {
	defer fmt.Printf("handleUserInput done\n")
	var wg sync.WaitGroup
	defer wg.Wait()
	input := bufio.NewScanner(src)
	for input.Scan() {
		if err := input.Err(); err != nil {
			log.Fatal("stdin: %s", err)
		}
		job := session.New(nil)
		job.Metadata.Set("content-type", "beam-job")
		job.NewRoute().HandleFunc(func(st *beam.Stream) {
			st.TailTo(os.Stdout, fmt.Sprintf("%s [%d] ", st.Parent(), st.Id()))
		})
		job.NewRoute().Headers("name", "stdout").HandleFunc(func(st *beam.Stream) {
			st.TailTo(os.Stdout, fmt.Sprintf("%s [stdout] ", st.Parent()))
		})
		job.NewRoute().Headers("name", "stderr").HandleFunc(func(st *beam.Stream) {
			st.TailTo(os.Stdout, fmt.Sprintf("%s [stderr] ", st.Parent()))
		})
		if err := job.Send(); err != nil {
			log.Fatalf("send: %s", err)
		}
		if _, err := job.Printf("%s\n", input.Text()); err != nil {
			log.Fatalf("write: %s", err)
		}
		wg.Add(1)
		go func(cmdline string) {
			err := job.TailTo(os.Stdout, fmt.Sprintf("[%d] [%s] ", job.Id(), cmdline))
			if err != nil {
				log.Printf("Error reading from stream: %s", err)
			}
			job.Close()
			fmt.Printf("[%d] [%s] Closed\n", job.Id(), cmdline)
			wg.Done()
		}(input.Text())
	}
}

// Server

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

func jobListen(job *jobs.Job) jobs.Status {
	if len(job.Args) < 2 {
		job.Stderr.Printf("Usage: %s PROTO ADDRESS\n", job.Name)
		return jobs.StatusErr
	}
	proto := job.Args[0]
	addr := job.Args[1]
	job.Stderr.Printf("Listening on %s/%s", proto, addr)
	l, err := net.Listen(proto, addr)
	if err != nil {
		job.Stderr.Printf("listen: %s\n", err)
		return jobs.StatusErr
	}
	for {
		conn, err := l.Accept()
		if err != nil {
			job.Stderr.Printf("accept: %s\n", err)
			return jobs.StatusErr
		}
		job.Stderr.Printf("New connection from %s\n", conn.RemoteAddr())
		st := job.New()
		fConn, hasFile := conn.(HasFile)
		if hasFile {
			f, err := fConn.File()
			if err != nil {
				job.Stderr.Printf("can't get connection file descriptor: %s", err)
				conn.Close()
				continue
			}
			st.SetFile(f)
		}
		// FIXME: since we're passing the socket file descriptor,
		// we can't intercept close events so st.Close will never be called.
		// This doesn't matter for the data channel, because the real fd itself will
		// be closed. However, if metadata is sent on a separate fd, how will that be closed?
		if err := st.Send(); err != nil {
			job.Stderr.Printf("send: %s\n", err)
			return jobs.StatusErr
		}
		if !hasFile {
			go func() {
				st.Printf("---> Splice\n")
				Splice(st, conn)
				conn.Close()
				st.Close()
			}()
		}
	}
	return jobs.StatusOK
}

type HasFile interface {
	File() (*os.File, error)
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

func jobExec(job *jobs.Job) jobs.Status {
	cmd := exec.Command("sh", "-c", strings.Join(job.Args, " "))
	cmd.Stdout = job.Stdout
	cmd.Stderr = job.Stderr
	if err := cmd.Run(); err != nil {
		job.Stderr.Printf("error: %s\n", err)
		return jobs.StatusErr
	}
	unixStatus, ok := cmd.ProcessState.Sys().(syscall.WaitStatus)
	if !ok {
		job.Stderr.Printf("error: exit status unavailable\n")
		return jobs.StatusErr
	}
	return jobs.Status(unixStatus.ExitStatus())
}


func jobDownload (job *jobs.Job) jobs.Status {
	if len(job.Args) < 1 {
		job.Stderr.Printf("Usage: %s URL\n", job.Name)
		return jobs.StatusErr
	}
	url := job.Args[0]
	job.Stderr.Printf("Downloading from %s\n", url)
	resp, err := http.Get(url)
	if err != nil {
		job.Stderr.Printf("GET %s: %s\n", url, err)
		return jobs.StatusErr
	}
	job.Stderr.Printf("%s\n", resp.Status)
	io.Copy(job.Stdout, resp.Body)
	return jobs.StatusOK
}

func jobEcho (job *jobs.Job) jobs.Status {
	job.Stdout.Printf("%#v\n", job.Args)
	return jobs.StatusOK
}

func jobCat (job *jobs.Job) jobs.Status {
	if len(job.Args) != 1 {
		job.Stderr.Printf("Usage: %s filename\n", job.Name)
		return jobs.StatusErr
	}
	f, err := os.Open(job.Args[0])
	if err != nil {
		job.Stderr.Printf("open: %s\n", err)
		return jobs.StatusErr
	}
	fStream := job.New()
	fStream.SetFile(f)
	fStream.Send()
	fStream.Close()
	return jobs.StatusOK
}
