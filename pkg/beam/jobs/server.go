package jobs

import (
	"github.com/dotcloud/docker/pkg/beam"
	"fmt"
	"strings"
)

type JobHandler func(*Job) Status

type Job struct {
	*beam.Stream
	Name   string
	Args   []string
	Stdout *beam.Stream
	Stderr *beam.Stream
}

type Status int

const (
	StatusOK       Status = 0
	StatusErr      Status = 1
	StatusNotFound Status = 127
)

func (h JobHandler) Send(st *beam.Stream) error {
	go h.handle(st)
	return nil
}

func (h JobHandler) handle(st *beam.Stream) error {
	job := &Job{
		Stream: st,
		Name:	st.Header().Get("name"),
		Args:   st.Header().GetAll("args"),
	}
	fmt.Printf("---> %s %s\n", job.Name, job.Args)
	job.Printf("job-name=%s\n", job.Name)
	job.Printf("job-args=%s\n", strings.Join(job.Args, "\x00"))
	job.Stdout = job.New()
	job.Stdout.Metadata.Set("name", "stdout")
	if err := job.Stdout.Send(); err != nil {
		return err
	}
	defer job.Stdout.Close()
	job.Stderr = job.New()
	job.Stderr.Metadata.Set("name", "stderr")
	if err := job.Stderr.Send(); err != nil {
		return err
	}
	defer job.Stderr.Close()
	status := h(job)
	if _, err := job.Printf("status=%d\n", status); err != nil {
		return err
	}
	return nil
}
