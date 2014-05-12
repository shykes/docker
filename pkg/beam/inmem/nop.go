package inmem

import (
	"io"
)

type NopSender struct{}

func (s NopSender) Send(msg *Message, mode int) (r Receiver, w Sender, err error) {
	if mode & R != 0 {
		r = NopReceiver{}
	}
	if mode & W != 0 {
		w = NopSender{}
	}
	return
}

func (s NopSender) Close() error {
	return nil
}

type NopReceiver struct{}

func (r NopReceiver) Receive(mode int) (*Message, Receiver, Sender, error) {
	return nil, nil, nil, io.EOF
}
