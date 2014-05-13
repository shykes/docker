package ur

import (
	"fmt"
	"github.com/dotcloud/docker/pkg/beam/data"
	"github.com/flynn/go-shlex"
	"io"
	"io/ioutil"
	"strings"
)

type Program struct {
	instructions []*Instruction
}

type Instruction struct {
	Name string
	Args []string
}

func Compile(src io.Reader) (*Program, error) {
	l, err := shlex.NewLexer(src)
	if err != nil {
		return nil, err
	}
	// FIXME: this creates a program with a single instruction,
	// spanning all words parsed from the input.
	// This is because shlex, as far as I know, does not support newlines.
	var cmd []string
	for {
		word, err := l.NextWord()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		cmd = append(cmd, word)
	}
	if len(cmd) == 0 {
		return nil, fmt.Errorf("parse error: empty command")
	}
	return NewProgram().Add(cmd[0], cmd[1:]...), nil
}

func NewProgram() *Program {
	return &Program{}
}

func (p *Program) Decode(bc io.Reader) (int, error) {
	// FIXME: decode the stream without buffering the whole thing first.
	data, err := ioutil.ReadAll(bc)
	if err != nil {
		return 0, err
	}
	var instructions []*Instruction
	for len(data) > 0 {
		inst, skip, err := decodeInstruction(string(data))
		if err != nil {
			return 0, err
		}
		instructions = append(instructions, inst)
		data = data[:skip]
	}
	p.instructions = append(p.instructions, instructions...)
	return len(instructions), nil
}

func (p *Program) Add(name string, args ...string) *Program {
	p.instructions = append(p.instructions, &Instruction{name, args})
	return p
}

func (p *Program) Reset() *Program {
	p.instructions = nil
	return p
}

func (p *Program) Encode(dst io.Writer) (int, error) {
	var n int
	for _, i := range p.instructions {
		chunk := i.Encode()
		if _, err := dst.Write([]byte(chunk)); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func (p *Program) String() string {
	var lines []string
	for _, i := range p.instructions {
		lines = append(lines, i.String())
	}
	return strings.Join(lines, "\n")
}

func (i *Instruction) String() string {
	return strings.Join(append([]string{i.Name}, i.Args...), " ")
}

func (i *Instruction) Encode() string {
	return data.EncodeList(append(
		[]string{i.Name},
		i.Args...,
	))
}

func decodeInstruction(bc string) (*Instruction, int, error) {
	words, skip, err := data.DecodeList(bc)
	if err != nil {
		return nil, 0, err
	}
	return &Instruction{words[0], words[1:]}, skip, nil
}
