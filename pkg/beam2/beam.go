package beam2

import (
	"fmt"
	"os"
	"github.com/dotcloud/docker/pkg/beam2/data"
)

const (
	O_CREATE int = os.O_CREATE
	O_EXCL  int = os.O_EXCL
)

// FIXME: how to represent nested jobs?

// ie. how do I represent contacting the docker engine to create more jobs?
// Do I represent the docker engine as a job?

type Handler func(stream *Stream)

type Session struct {
	transport Transport
	streams map[string]Stream
}

func New(transport Transport) *Session {
	return &Session{
		transport: transport,
		streams: make(map[string]Stream),
	}
}

func (s *Session) Streams() []string {
	names := make([]string, 0, len(s.streams))
	for name, _ := range(s.streams) {
		names = append(names, name)
	}
	return names
}

// The following flags can be passed to Open:
// O_CREATE: create the stream if it doesn't exist.
// O_EXCL: error if O_CREATE and stream exists.
func (s *Session) Open(name string, flag int) (stream Stream, err error) {
	if st, exists := s.streams[name]; exists {
		if flag & (O_CREATE | O_EXCL) != 0 {
			return nil, fmt.Errorf("stream already exists: %s", name)
		}
		stream = st
	} else {
		if flag & O_CREATE == 0 {
			return nil, fmt.Errorf("no such stream: %s", name)
		}
		// Create a new stream
		stream, err = s.transport.SendStream(flag)
		if err != nil {
			return
		}
	}
	return
}

func (s *Session) Close() error {
	for _, stream := range s.streams {
		stream.Close()
	}
	return s.transport.Close()
}

type Transport interface {
	SendStream(flag int) (Stream, error)
	ReceiveStream(flag int) (Stream, error)
	Close() error
}


type Stream interface {
	// Send sends a message on the stream. The underlying transport must
	// guarantee that message boundaries and ordering are preserved.
	Send(Frame) error

	// Receive blocks until a message is available on the stream and returns it.
	Receive() (Frame, error)

	// Close closes the stream, rendering it unusable for I/O. It returns an
	// error, if any.
	Close() error

	// Metadata returns an endpoint for sending and receiving metadata for this stream.
	// Metadata is sent of the form of messages. Each message holds a collection of key=value pairs.
	// the same pair may be defined multiple times in the same message.
	//
	// FIXME: specify if Metadata returns a singleton, and if not, how multiple instances
	// are handled.
	//
	Metadata() data.StructuredStream

	// Hijack lets the caller take over the stream if the underlying transport allows it,
	// or returns an error if it doesn't.
	//
	// If successful, Hijack returns the file descriptor for the stream, and the transport
	// will no to anything else with it. It is then the caller's responsibility to manage
	// and close the file descriptor.
	//
	Hijack() (fd int, err error)
}

type Frame []byte
