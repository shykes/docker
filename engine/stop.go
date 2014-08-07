package engine

import (
	"sync"
)

// StopHandler is a utility for processing concurrent request to stop something.
// It is used to implement Job.Stop and Job.OnStop
type StopHandler struct {
	msgs     chan *msgStop
	handlers chan func()
	l        sync.RWMutex
}

func NewStopHandler() *StopHandler {
	s := &StopHandler{
		msgs:     make(chan *msgStop),
		handlers: make(chan func()),
	}
	go processStops(s.msgs, s.handlers)
	return s
}

type msgStop struct {
	resp chan struct{}
}

func newMsgStop() *msgStop {
	return &msgStop{
		resp: make(chan struct{}),
	}
}

func (s *StopHandler) Teardown() {
	s.l.Lock()
	defer s.l.Unlock()
	close(s.msgs)
	s.msgs = nil
	close(s.handlers)
	s.handlers = nil
}

func (s *StopHandler) Stop() {
	s.l.RLock()
	defer s.l.RUnlock()
	if s.msgs == nil {
		return
	}
	msg := newMsgStop()
	s.msgs <- msg
	<-msg.resp
	return
}

func (s *StopHandler) OnStop(h func()) {
	s.l.RLock()
	defer s.l.RUnlock()
	if s.handlers == nil {
		return
	}
	s.handlers <- h
}

// processStops is expected to run in a singleton goroutine at initialization
// of the Job. It receives messages from Shutdown() and OnShutdown().
//
// When Run() completes, it closes job.shutdownMsgs and job.shutdownHandlers,
// which causes processStops to return.
// Consequently, calling Shutdown or OnShutdown after Run returns successfully
// will do nothing.
func processStops(msgs chan *msgStop, handlers chan func()) {
	var onStop func() // the handler to call
	backlog := make([]*msgStop, 0)
	call := make(chan struct{})
	flushBacklog := func() {
		for _, msg := range backlog {
			if msg.resp != nil {
				close(msg.resp)
			}
		}
		backlog = nil
	}
	callHandler := func() {
		onStop()
		call <- struct{}{}
	}
processLoop:
	for {
		select {
		// New stop request
		case msg, ok := <-msgs:
			{
				if !ok {
					break processLoop
				}
				// Handler already called: immediate response
				if backlog == nil {
					close(msg.resp)
					continue
				}
				backlog = append(backlog, msg)
				if onStop != nil {
					go callHandler()
				}
			}
		// Stop handler has completed
		case <-call:
			{
				flushBacklog()
			}
		// New handler
		case h, ok := <-handlers:
			{
				if !ok {
					break processLoop
				}
				onStop = h
				if len(backlog) > 0 {
					go callHandler()
				}
			}
		}
	}
	flushBacklog()
}
