package engine

import (
	"testing"
	"time"

	_ "github.com/docker/docker/pkg/testutils"
)

func TestStopAfterRegister(t *testing.T) {
	eng := New()
	registerStopDone := make(chan struct{})
	var called bool
	eng.Register("infinite", func(job *Job) Status {
		job.OnStop(func() {
			called = true
		})
		close(registerStopDone)
		<-make(chan struct{})
		return StatusOK
	})
	infinite := eng.Job("infinite")
	go infinite.Run()
	<-registerStopDone
	infinite.Stop()
	if !called {
		t.Fatalf("stop handler not called")
	}
}

func TestStopBeforeRegister(t *testing.T) {
	eng := New()
	var called bool
	started := make(chan struct{})
	stopSent := make(chan struct{})
	stopped := make(chan struct{})
	eng.Register("infinite", func(job *Job) Status {
		close(started)
		<-stopSent
		job.OnStop(func() {
			called = true
		})
		<-make(chan struct{})
		return StatusOK
	})
	infinite := eng.Job("infinite")
	go infinite.Run()
	<-started
	go func() {
		infinite.Stop()
		close(stopped)
	}()
	// Wait long enough for Stop() to be called *before*
	// we register the handler
	time.Sleep(200 * time.Millisecond)
	close(stopSent)
	<-stopped
	if !called {
		t.Fatalf("stop handler not called")
	}
}
