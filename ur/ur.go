package ur

import (
	"bytes"
	"fmt"
	beam "github.com/dotcloud/docker/pkg/beam/inmem"
	"strings"
	"sync"
)

type Runtime struct {
	id Id
}

func New(id Id) *Runtime {
	return &Runtime{
		id: id,
	}
}

type Builtin func(*beam.Message, beam.Receiver, beam.Sender, beam.Sender) error

func (rt *Runtime) Close() error {
	return nil
}

func (rt *Runtime) Send(msg *beam.Message, mode int) (beam.Receiver, beam.Sender, error) {
	var h Builtin
	switch msg.Name {
	case "eval":
		h = rt.opEval
	case "print":
		h = rt.opPrint
	default:
		h = rt.opNotFound
	}

	inr, inw := beam.Pipe()
	if mode&beam.W == 0 {
		inw.Close()
	}
	outr, outw := beam.Pipe()
	if mode&beam.R == 0 {
		outr.Close()
	}
	go func() {
		err := h(msg, inr, outw, rt)
		// FIXME: implement error passing
		if err != nil {
			outw.Send(&beam.Message{"error", []string{err.Error()}, nil}, 0)
		}
		outw.Close()
		inr.Close()
	}()
	return outr, inw, nil
}

func (rt *Runtime) opNotFound(msg *beam.Message, in beam.Receiver, out beam.Sender, caller beam.Sender) error {
	caller.Send(&beam.Message{"error", []string{fmt.Sprintf("command not found: %s", msg.Name)}, nil}, 0)
	return nil
}

func (rt *Runtime) opPrint(msg *beam.Message, in beam.Receiver, out beam.Sender, caller beam.Sender) error {
	fmt.Printf("%s\n", strings.Join(msg.Args, " "))
	return nil
}

func (rt *Runtime) opEval(msg *beam.Message, in beam.Receiver, out beam.Sender, caller beam.Sender) error {
	out.Send(&beam.Message{"log", []string{"starting eval"}, nil}, 0)
	if len(msg.Args) != 1 {
		return fmt.Errorf("usage: %s BYTECODE", msg.Name)
	}
	p := NewProgram()
	n, err := p.Decode(strings.NewReader(msg.Args[0]))
	if err != nil {
		out.Send(&beam.Message{"error", []string{fmt.Sprintf("decode: %v", err)}, nil}, 0)
		return err
	}
	out.Send(&beam.Message{"log", []string{fmt.Sprintf("[eval] parsed %d instructions", n)}, nil}, 0)
	for _, i := range p.Instructions() {
		r, w, err := caller.Send(&beam.Message{i.Name, i.Args, nil}, beam.R|beam.W)
		if err != nil {
			return err
		}
		var tasks sync.WaitGroup
		tasks.Add(2)
		go func() {
			beam.Copy(out, r)
			tasks.Done()
		}()
		go func() {
			beam.Copy(w, in)
			tasks.Done()
		}()
		tasks.Wait()
	}
	return nil
}

// A unique identifier for this runtime.
func (rt *Runtime) Id() Id {
	return rt.id
}

// Each runtime instruction is a runtime method.
// * under the hood it can translate to a beam message
// * if beam implements a convenience rpc layer, we would use it as a middle layer
// * maybe expose a raw beam endpoint for power users? (but not for now)

type Service struct {
	beam.Receiver
	beam.Sender
}

func (s *Service) Start() error {
	_, _, err := s.Send(&beam.Message{"start", nil, nil}, 0)
	return err
}

func (s *Service) Stop() error {
	_, _, err := s.Send(&beam.Message{"stop", nil, nil}, 0)
	return err

}

func (s *Service) Eval(p *Program) (*Service, error) {
	bc := new(bytes.Buffer)
	p.Encode(bc)
	r, w, err := s.Send(&beam.Message{"eval", []string{bc.String()}, nil}, beam.R|beam.W)
	return &Service{r, w}, err
}

func (s *Service) Print(text string) error {
	_, _, err := s.Send(&beam.Message{"print", []string{text}, nil}, 0)
	return err
}

//func (s *Service) Prompt(key string) (SyncReader, error) {
// FIXME: this call doesn't have to be synchronous. Instead of blocking
// until the key is returned, return a feed from which future valuues
// of the key can be read. Furthermore, make that feed implement a standard
// sync interface so that it can be piped into other calls implementing it.
func (s *Service) Prompt(key string) (string, error) {
	r, _, err := s.Send(&beam.Message{"prompt", []string{key}, nil}, beam.R)
	if err != nil {
		return "", err
	}
	defer r.Close()
	msg, _, _, err := r.Receive(0)
	if err != nil {
		return "", err
	}
	if msg.Name != "set" {
		return "", fmt.Errorf("unexpected response: '%s'", msg.Name)
	}
	return strings.Join(msg.Args, ""), nil
}

func (s *Service) Set(key, value string) error {
	_, _, err := s.Send(&beam.Message{"set", []string{key, value}, nil}, 0)
	return err
}

func (s *Service) Exec(name string, args ...string) (*Service, error) {
	r, w, err := s.Send(&beam.Message{"exec", append([]string{name}, args...), nil}, beam.R|beam.W)
	if err != nil {
		return nil, err
	}
	return &Service{r, w}, nil
}

func (s *Service) Mkdir(dir, mode string) error {
	_, _, err := s.Send(&beam.Message{"mkdir", []string{dir, mode}, nil}, 0)
	return err
}

func (s *Service) Rmdir(dir string) error {
	_, _, err := s.Send(&beam.Message{"rm", []string{dir}, nil}, 0)
	return err
}

func (s *Service) If(cond, onTrue, onFalse string) (*Service, error) {
	r, w, err := s.Send(&beam.Message{"if", []string{cond, onTrue, onFalse}, nil}, beam.R|beam.W)
	if err != nil {
		return nil, err
	}
	return &Service{r, w}, nil
}

func (s *Service) While(cond, onTrue string) (*Service, error) {
	r, w, err := s.Send(&beam.Message{"while", []string{cond, onTrue}, nil}, beam.R|beam.W)
	if err != nil {
		return nil, err
	}
	return &Service{r, w}, nil
}

func (s *Service) Auth(id Id) error {
	challenge, _, err := s.Send(&beam.Message{"auth", []string{id.String()}, nil}, beam.R)
	if err != nil {
		return err
	}
	defer challenge.Close()
	msg, _, w, err := challenge.Receive(beam.W)
	if err != nil {
		return err
	}
	defer w.Close()
	if msg.Name != "challenge" {
		return fmt.Errorf("unexpected command: '%s'", msg.Name)
	}
	if len(msg.Args) != 1 {
		// FIXME: also send an error to the challenger
		return fmt.Errorf("invalid challenge")
	}
	response, err := id.Sign(msg.Args[0])
	if err != nil {
		return err
	}
	if _, _, err = w.Send(&beam.Message{"", []string{response}, nil}, 0); err != nil {
		return err
	}
	return nil
}

func (s *Service) sendrw(name string, args ...string) (*Service, error) {
	r, w, err := s.Send(&beam.Message{name, args, nil}, beam.R|beam.W)
	if err != nil {
		return nil, err
	}
	return &Service{r, w}, nil
}

func (s *Service) Serve(name string, bc string) (*Service, error) {
	return s.sendrw("serve", name, bc)
}

func (s *Service) Pipeline(bc ...string) (*Service, error) {
	return s.sendrw("pipeline", bc...)
}

// Connect establishes a connection with the service available at `name`.
// `name` is always relative to the current endpoint.
func (s *Service) Attach(name string) (*Service, error) {
	return s.sendrw("attach", name)
}

// If s supports local execution, it returns the bytecode necessary
// for the client to execute it.
func (s *Service) Pull() (*Service, error) {
	return s.sendrw("pull")
}

func (s *Service) Install(bc string) error {
	_, _, err := s.Send(&beam.Message{"install", []string{bc}, nil}, 0)
	return err
}
