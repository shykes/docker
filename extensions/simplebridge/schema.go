package simplebridge

import (
	"fmt"
	"path"

	"github.com/docker/docker/state"
)

type Schema struct {
	path  []string
	state state.State
}

const (
	NetworkPrefix  = "networks"
	EndpointPrefix = "endpoints"
)

func NewSchema(thisState state.State) *Schema {
	return &Schema{
		path:  []string{},
		state: thisState,
	}
}

func (s *Schema) Scope(path string) *Schema {
	return &Schema{
		path:  append(s.path, path),
		state: s.state,
	}
}

func (s *Schema) Network(name string) *Schema {
	return s.Scope(NetworkPrefix).Scope(name)
}

func (s *Schema) Endpoint(network, endpoint string) *Schema {
	return s.Scope(EndpointPrefix).Scope(network).Scope(endpoint)
}

func (s *Schema) Join(newpath string) string {
	prefix := path.Join(s.path...)

	if newpath != "" {
		return path.Join(prefix, newpath)
	}

	return prefix
}

func (s *Schema) Create(path string) error {
	return s.state.Mkdir(s.Join(path))
}

func (s *Schema) Remove(path string) error {
	return s.state.Remove(s.Join(path))
}

func (s *Schema) Get(path string) (string, error) {
	return s.state.Get(s.Join(path))
}

func (s *Schema) Set(path string, value string) error {
	return s.state.Set(s.Join(path), value)
}

func (s *Schema) MultiSet(pathMap map[string]string) error {
	for path, value := range pathMap {
		err := s.state.Set(s.Join(path), value)
		if err != nil {
			return fmt.Errorf("Error setting property %q: %v", path, err)
		}
	}

	return nil
}

func (s *Schema) Exists(path string) bool {
	_, err := s.state.Get(s.Join(path))
	return err == nil
}
