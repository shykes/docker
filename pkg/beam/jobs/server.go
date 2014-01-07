package jobs

import (
	"bufio"
	"github.com/dotcloud/docker/pkg/beam"
	"fmt"
	"strings"
)

type Server struct {
	handlers map[string]Handler
}

type Job struct {
	*beam.Stream
	Name   string
	Args   []string
	Stdout *beam.Stream
	Stderr *beam.Stream
}

type Handler func(*Job) Status

type Status int

const (
	StatusOK       Status = 0
	StatusErr      Status = 1
	StatusNotFound Status = 127
)

func NewServer() *Server {
	return &Server{
		handlers: make(map[string]Handler),
	}
}

func (srv *Server) Bind(sessions ...*beam.Session) *Server {
	for _, s := range sessions {
		r := s.NewRoute()
		r.Parent().Headers("content-type", "beam-job").HandleFunc(srv.handleStream)
	}
	return srv
}

func (srv *Server) Register(name string, handler Handler) *Server {
	srv.handlers[name] = handler
	return srv
}

func (srv *Server) handleStream(st *beam.Stream) {
	fmt.Printf("---> New job\n")
	scanner := bufio.NewScanner(st)
	scanner.Scan()
	if err := scanner.Err(); err != nil {
		return
	}
	fmt.Printf("---> %s\n", scanner.Text())
	words := strings.Split(strings.Trim(scanner.Text(), " \t"), " ")
	job := &Job{
		Stream: st,
		Name:   words[0],
		Args:   words[1:],
	}
	job.Printf("job-name=%s\n", job.Name)
	job.Printf("job-args=%s\n", strings.Join(job.Args, "\x00"))
	job.Stdout = job.New()
	job.Stdout.Metadata.Set("name", "stdout")
	if err := job.Stdout.Send(); err != nil {
		return
	}
	defer job.Stdout.Close()
	job.Stderr = job.New()
	job.Stderr.Metadata.Set("name", "stderr")
	if err := job.Stderr.Send(); err != nil {
		return
	}
	defer job.Stderr.Close()
	handler, exists := srv.handlers[job.Name]
	if !exists {
		handler = srv.notFoundHandler
	}
	status := handler(job)
	job.Printf("status=%d\n", status)
}

func (srv *Server) notFoundHandler(job *Job) Status {
	job.Stderr.Printf("no such command: %s\n", job.Name)
	return StatusNotFound
}
