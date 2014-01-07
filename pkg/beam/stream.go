package beam

import (
	"bufio"
	"fmt"
	"github.com/dotcloud/docker/pkg/beam/data"
	"io"
	"os"
)

type Stream struct {
	id         uint32
	parent     *Stream
	Metadata   data.Msg
	local      *os.File
	remote     *os.File
	metaLocal  *os.File
	metaRemote *os.File
	session    *Session
	// FIXME: this isn't needed once Server is merged into Session
	onId       []func(int)
	chErr      chan error
}

func (s *Stream) OnId(fn func(int)) {
	s.onId = append(s.onId, fn)
}

func (s *Stream) Send() error {
	// If no file has been set with SetFile, setup a socketpair.
	if s.remote == nil {
		local, remote, err := Socketpair()
		if err != nil {
			return fmt.Errorf("socketpair: %v", err)
		}
		s.SetFile(remote)
		s.local = local
	}
	s.chErr = make(chan error)
	s.session.chSend <-s
	return <-s.chErr
}

func (s *Stream) New() *Stream {
	return s.session.New(s)
}

func (s *Stream) infoMsg() data.Msg {
	info := make(data.Msg)
	info.SetInt("id", int64(s.id))
	if p := s.Parent(); p != nil {
		info.SetInt("parent-id", int64(p.Id()))
	}
	// Send initial metadata, if any, as a nested "header" field
	if len(s.Metadata) > 0 {
		info.Set("header", s.Metadata.String())
	}
	return info
}

func (s *Stream) SetFile(f *os.File) {
	s.remote = f
	s.local = nil
}

func (s *Stream) GetFile() (f *os.File, err error) {
	if s.local != nil {
		return f, nil
	}
	return nil, fmt.Errorf("local endpoint not available")
}

func (s *Stream) Read(d []byte) (int, error) {
	if s.local == nil {
		return 0, fmt.Errorf("read: local endpoint not available")
	}
	return s.local.Read(d)
}

func (s *Stream) Write(d []byte) (int, error) {
	if s.local == nil {
		return 0, fmt.Errorf("write: local endpoint not available")
	}
	return s.local.Write(d)
}

func (s *Stream) Printf(format string, args ...interface{}) (int, error) {
	return fmt.Fprintf(s, format, args...)
}

func (s *Stream) TailTo(dst io.Writer, prefix string) error {
	scanner := bufio.NewScanner(s)
	for scanner.Scan() {
		if line := scanner.Text(); line != "" {
			if _, err := fmt.Fprintf(dst, "%s%s\n", prefix, line); err != nil {
				return err
			}
		}
		if err := scanner.Err(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Stream) Close() error {
	if s.local == nil {
		return fmt.Errorf("close: local endpoint not available")
	}
	s.local.Sync()
	return s.local.Close()
}

func (s *Stream) Id() int {
	return int(s.id)
}

func (s *Stream) Parent() *Stream {
	return s.parent
}

func (s *Stream) String() string {
	var fd uintptr
	if s.local != nil {
		fd = s.local.Fd()
	}
	return fmt.Sprintf("(id=%d fd=%d headers=%s)", s.Id(), fd, s.Metadata.ShortString())
}
