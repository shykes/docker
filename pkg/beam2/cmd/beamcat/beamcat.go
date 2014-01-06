package main

import (
	"io"
	"os"
	"bufio"
	"os/exec"
	"fmt"
	"net"
	"sync"
	"time"
	"strings"
	"net/http"
	"log"
	beam "github.com/dotcloud/docker/pkg/beam2"
)

func main() {
	sock, server, err := connectStream(".beam.sock")
	if err != nil {
		log.Fatal(err)
	}
	session := beam.New(sock, server)
	defer session.Close()
	srv := NewServer(session)
	newJobs := srv.NewRoute()
	newJobs.Parent()
	newJobs.Headers("content-type", "beam-job")
	newJobs.HandleFunc(handleNewJob)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		handleUserInput(os.Stdin, srv)
		wg.Done()
	}()
	go func() {
		if err := srv.Serve(); err != nil {
			fmt.Fprintf(os.Stderr, "serve: %s\n", err)
		}
		wg.Done()
	}()
	wg.Wait()
}

func handleNewJob(st *beam.Stream) {
	fmt.Printf("---> New job\n")
	scanner := bufio.NewScanner(st)
	scanner.Scan()
	if err := scanner.Err(); err != nil {
		log.Fatal("read from peer: %s", err)
	}
	fmt.Printf("---> %s\n", scanner.Text())
	words := strings.Split(strings.Trim(scanner.Text(), " \t"), " ")
	job := &Job{
		Stream: st,
		Name: words[0],
		Args: words[1:],
	}
	job.Printf("job-name=%s\n", job.Name)
	job.Printf("job-args=%s\n", strings.Join(job.Args, "\x00"))
	job.Stdout = job.New()
	job.Stdout.Metadata.Set("name", "stdout")
	if err := job.Stdout.Send(); err != nil {
		log.Fatalf("send stdout: %s", err)
	}
	job.Stderr = job.New()
	job.Stderr.Metadata.Set("name", "stderr")
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
	// FIXME: WHY DOES Stream.local.Sync() not fix the race condition arggghhh
	time.Sleep(42 * time.Millisecond)
	job.Stdout.Close()
	job.Stderr.Close()
	job.Close()
}


func handleUserInput(src io.Reader, srv *Server) {
	defer fmt.Printf("handleUserInput done\n")
	var wg sync.WaitGroup
	defer wg.Wait()
	input := bufio.NewScanner(src)
	for input.Scan() {
		if err := input.Err(); err != nil {
			log.Fatal("stdin: %s", err)
		}
		job := srv.Session().New(nil)
		job.Metadata.Set("content-type", "beam-job")
		{
			cmdline := input.Text()
			err := job.Send(func(id int) {
				srv.NewRoute().Parent(id).Headers("name", "stdout").HandleFunc(func(st *beam.Stream) {
					st.TailTo(os.Stdout, fmt.Sprintf("[%d/stdout] [%s] ", job.Id(), cmdline))
				})
				srv.NewRoute().Parent(id).Headers("name", "stderr").HandleFunc(func(st *beam.Stream) {
					st.TailTo(os.Stderr, fmt.Sprintf("[%d/stderr] [%s] " , job.Id(), cmdline))
				})
			})
			if err != nil {
				log.Fatalf("send: %s", err)
			}
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

type Server struct {
	session *beam.Session
	routes []*Route
}

func (srv *Server) Session() *beam.Session {
	return srv.session
}

func NewServer(session *beam.Session) *Server {
	return &Server{session: session}
}

func (srv *Server) NewRoute() *Route {
	route := &Route{}
	srv.routes = append(srv.routes, route)
	return route
}

type Route struct {
	filters []func(*beam.Stream) bool
	fn func(*beam.Stream)
}

func (r *Route) Parent(parentIds ...int) *Route {
	r.filters = append(r.filters, func(st *beam.Stream) (match bool) {
		parent := st.Parent()
		if parent == nil  {
			return len(parentIds) == 0
		}
		for _, parentId := range parentIds {
			if parent.Id() == parentId {
				return true
			}
		}
		return false
	})
	return r
}

func (r *Route) Headers(pairs ...string) *Route {
	r.filters = append(r.filters, func(st *beam.Stream) (match bool) {
		for i:=0; i < len(pairs); i+=2 {
			key := pairs[i]
			var value string
			if len(pairs) > i + 1 {
				value = pairs[i + 1]
			}
			if value == "" {
				if !st.Metadata.Exists(key) {
					return false
				}
				continue
			}
			if st.Metadata.Get(key) != value {
				return false
			}
		}
		return true
	})
	return r
}

func (r *Route) HandleFunc(fn func(*beam.Stream)) *Route {
	r.fn = fn
	return r
}

func (r *Route) Match(st *beam.Stream) (match bool) {
	for _, filter := range r.filters {
		if filter(st) == false {
			return false
		}
	}
	return true
}

func (r *Route) Handle(st *beam.Stream) {
	if r.fn == nil {
		return
	}
	r.fn(st)
}

func (srv *Server) Serve() error {
	var wg sync.WaitGroup
	defer wg.Wait()
	for {
		st, err := srv.session.Receive()
		if err != nil {
			return fmt.Errorf("receive: %s", err)
		}
		fmt.Printf("+++ %d %s\n", st.Id(), st.Metadata.ShortString())
		for i := range srv.routes {
			// Last route added wins
			route := srv.routes[len(srv.routes) - i - 1]
			if route.Match(st) {
				wg.Add(1)
				go func() {
					route.Handle(st)
					st.Close()
					wg.Done()
				}()
				continue
			}
		}
		fmt.Printf("No matching route for inbound stream %d. Dropping\n", st.Id())
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
			job.Printf("status=1\n")
			return
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
	*beam.Stream
	Name string
	Args []string
	Stdout *beam.Stream
	Stderr *beam.Stream
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
