package main

import (
	"bufio"
	"fmt"
	"github.com/dotcloud/docker/pkg/beam"
	"github.com/dotcloud/docker/pkg/beam/jobs"
	"io"
	"io/ioutil"
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
	r := beam.NewRouter()
	r.NewRoute().Name("download").Handler(jobs.JobHandler(jobDownload))
	r.NewRoute().Name("listen").Handler(jobs.JobHandler(jobListen))
	r.NewRoute().Name("exec").Handler(jobs.JobHandler(jobExec))
	r.NewRoute().Name("echo").Handler(jobs.JobHandler(jobEcho))
	r.NewRoute().Name("cat").Handler(jobs.JobHandler(jobCat))
	newJobs := make(map[string]*beam.Stream)
	r.NewRoute().Name("register").HandleFunc(func(st *beam.Stream) {
		for _, name := range st.Header().GetAll("args") {
			fmt.Printf("Registering %s for %s\n", st, name)
			newJobs[name] = st
		}
	})
	r.NewRoute().MatcherFunc(func (st *beam.Stream) bool {
		_, exists := newJobs[st.Header().Get("name")]
		return exists
	}).HandleFunc(func(st *beam.Stream) {
		worker := newJobs[st.Header().Get("name")]
		dest := worker.New()
		dest.Header().Set("name", st.Header().Get("name"))
		dest.Header().Set("args", st.Header().GetAll("args")...)
		if f, err := dest.GetFile(); err != nil {
			go func() {
				Splice(dest, st)
				dest.Close()
			}()
		} else {
			dest.SetFile(f)
		}
		dest.Send()
	})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		//beam.CopyStreams(r, session)
		r.ReceiveFrom(session)
		wg.Done()
	}()
	go func() {
		handleUserInput(os.Stdin, session)
		wg.Done()
	}()
	wg.Wait()
}

// FIXME encode job name & args into headers, to merge job routing & stream routing
// (eg a job can be routed simply with .Headers("job-name", "exec")
//
// FIXME merge headers "job-name" and "name". Maybe a job is nothing more than a stream
// with a name. A convention for dependency injection, whether it's "stdout", "exec" or "db"
//
// FIXME: use Send/Receive as boundary instead of callbacks. You can always wrap a callback into
// 


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
		words := strings.Split(strings.Trim(input.Text(), " \t"), " ")
		job.Metadata.Set("name", words[0])
		job.Metadata.Set("args", words[1:]...)
		router := beam.NewRouter()
		router.NewRoute().HandleFunc(func(st *beam.Stream) {
			st.TailTo(os.Stdout, fmt.Sprintf("%s [%d] ", st.Parent(), st.Id()))
		})
		router.NewRoute().Headers("name", "stdout").HandleFunc(func(st *beam.Stream) {
			st.TailTo(os.Stdout, fmt.Sprintf("%s [stdout] ", st.Parent()))
		})
		router.NewRoute().Headers("name", "stderr").HandleFunc(func(st *beam.Stream) {
			st.TailTo(os.Stderr, fmt.Sprintf("%s [stderr] ", st.Parent()))
		})
		wg.Add(2)
		go func() {
			io.Copy(ioutil.Discard, job)
			job.Close()
			fmt.Printf("%s closed\n", job)
			wg.Done()
		}()
		go func() {
			router.ReceiveFrom(job)
			router.Close()
			wg.Done()
		}()
		if err := job.Send(); err != nil {
			log.Fatalf("send: %s", err)
		}
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
	job.Stderr.Printf("Listening on %s/%s\n", proto, addr)
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
		/*
		if !hasFile {
			go func() {
				st.Printf("---> Splice\n")
				Splice(st, conn)
				conn.Close()
				st.Close()
			}()
		}
		*/
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
