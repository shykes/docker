package ur

import (
	beam "github.com/dotcloud/docker/pkg/beam/inmem"
)

type Hub struct {
	sync.RWMutex
	handler beam.Sender
}

func NewHub() *Hub {
	return &Hub{
		handler: NopSender{},
	}
}

func (hub *Hub) Serve(src beam.Receiver) error {
	var tasks sync.WaitGroup
	for {
		msg, msgr, msgw, err := src.Receive(beam.R|beamW)
		if err != nil {
			return err
		}
		if msg.Name == "register" {
			// Requests from the new handler are passed to the next handler
			go func(nextHandler beam.Sender, msgr beam.Receiver) {
				beam.Copy(nextHandler, msgr)
			}(hub.handler, msgr)
			// Future requests are passed to the current handler.
			// if the handler stops responding, acquire the lock and 
			// --> USE A LINKED LIST
			// AND MAY THE FORCE BE WITH YOU
		}
	}
}


func (hub *Hub) Send(msg *beam.Message, mode int) (beam.Receiver, beam.Sender, error) {
	if msg.Name == "register" {
		outr, outw := beam.Pipe()
		inr, inw := beam.Pipe()
		hub.Lock()
		defer hub.Unlock()
		oldHandler := hub.handler
		hub.handler = inw
		go func() {
			// New messages will arrive through:
			// hub.handler=inw -> inr -> outw -> outr -> (registered handler)
			beam.Copy(outw, inr)
			hub.Lock()
			defer hub.Unlock
		}()
	}
}


type NopSender struct {}

func (s NopSender) Send(msg *beam.Message, mode int) (beam.Receiver, beam.Sender, error) {
	return NopReceiver{}, NopSender{}, nil
}

type NopReceiver struct{}

func (r NopReceiver) Receive(mode int) (*beam.Message, beam.Receiver, beam.Sender, error) {
	return nil, nil, nil, io.EOF
}
