package ur

import (
	"fmt"
)

type Id interface {
	String() string
	Sign(data string) (string, error)
}

type DummyId string

func (id DummyId) String() string {
	return string(id)
}

func (id DummyId) Sign(data string) (string, error) {
	return "", fmt.Errorf("not implemented")
}
