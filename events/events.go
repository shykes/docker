package events

import (
	"encoding/json"
	"fmt"
	"github.com/dotcloud/docker/engine"
	"github.com/dotcloud/docker/utils"
	"sync"
	"time"
)

type Logger struct {
	sync.RWMutex
	listeners map[string]chan utils.JSONMessage
	events    []utils.JSONMessage
}

func NewLogger() *Logger {
	return &Logger{
		listeners: make(map[string]chan utils.JSONMessage),
	}
}

func (l *Logger) Install(eng *engine.Engine) error {
	eng.Register("events", l.Events)
	eng.Register("logevent", l.LogEvent)
	eng.Register("events_info", l.Info)
	return nil
}

func (l *Logger) Info(job *engine.Job) engine.Status {
	info := &engine.Env{}
	info.SetInt("NEventsListener", len(l.listeners))
	if _, err := info.WriteTo(job.Stdout); err != nil {
		return job.Error(err)
	}
	return engine.StatusOK
}

func (l *Logger) LogEvent(job *engine.Job) engine.Status {
	if len(job.Args) != 3 {
		return job.Errorf("usage: %s ACTION ID FROM", job.Name)
	}
	jm := utils.JSONMessage{
		// FIXME: why Status and not Action?
		Status: job.Args[0],
		ID:     job.Args[1],
		From:   job.Args[2],
		Time:   time.Now().UTC().Unix(),
	}
	l.Lock()
	fmt.Printf("LogEvent got lock\n")
	l.events = append(l.events, jm)
	for _, c := range l.listeners {
		select { // non blocking channel
		case c <- jm:
		default:
		}
	}
	fmt.Printf("LogEvent releasing lock\n")
	l.Unlock()
	return engine.StatusOK
}

func (l *Logger) Events(job *engine.Job) engine.Status {
	if len(job.Args) != 1 {
		return job.Errorf("Usage: %s FROM", job.Name)
	}

	var (
		from = job.Args[0]
	)
	sendEvent := func(event *utils.JSONMessage) error {
		b, err := json.Marshal(event)
		if err != nil {
			return err
		}
		_, err = job.Stdout.Write(b)
		if err != nil {
			return err
		}
		return nil
	}

	listener := make(chan utils.JSONMessage)
	l.Lock()
	fmt.Printf("Events got lock\n")
	if old, ok := l.listeners[from]; ok {
		delete(l.listeners, from)
		close(old)
	}
	l.listeners[from] = listener
	fmt.Printf("Events releasing lock\n")
	l.Unlock()
	// Remove our listener when we're done
	defer func() {
		l.Lock()
		delete(l.listeners, from)
		l.Unlock()
	}()
	// Notify the caller that all future events will be received.
	// This allows the caller to synchronize event consumption with
	// other threads (for example in unit tests).
	// FIXME: use beam primitives to send all results in a response stream.
	// Once we do that, the sending the of the response stream doubles
	// as a synchronization event.
	fmt.Printf("waiting 1 second before indicating sync-ready...\n")
	time.Sleep(1 * time.Second)
	fmt.Printf("indicating sync-ready\n")
	job.Stdout.Write([]byte(" "))
	for event := range listener {
		fmt.Printf("Events -> %v\n", event)
		err := sendEvent(&event)
		if err != nil && err.Error() == "JSON error" {
			continue
		}
		if err != nil {
			return job.Error(err)
		}
	}
	return engine.StatusOK
}

func (l *Logger) getEvents() []utils.JSONMessage {
	l.RLock()
	defer l.RUnlock()
	return l.events
}
