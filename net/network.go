package net

import (
	"github.com/docker/docker/extensions"
	"github.com/docker/docker/utils"
)

type Container interface {
	NSPath() string
}

type Endpoint struct {
	container  Container
	id         string
	interfaces []*extensions.Interface
	name       string
}

func NewNetwork(id string, extension extensions.NetExtension, state extensions.State) *Network {
	return &Network{
		id:        id,
		endpoints: make(map[string]*Endpoint),
		extension: extension,
		state:     state,
	}
}

type Network struct {
	id        string
	endpoints map[string]*Endpoint
	extension extensions.NetExtension
	state     extensions.State
}

// Create an endpoint with specified name in the network. The provided network
// namespace is where extensions created interfaces will be moved to.
func (n *Network) AddEndpoint(container Container, name string) error {
	id := utils.GenerateRandomID()
	interfaces, err := n.extension.AddEndpoint(n.id, id, n.state)
	if err != nil {
		return err
	}

	endpoint := &Endpoint{
		container:  container,
		id:         id,
		interfaces: interfaces,
		name:       name,
	}
	if err := endpoint.Configure(); err != nil {
		return err
	}

	n.endpoints[id] = endpoint
	return nil
}

func (e *Endpoint) Configure() error {
	for _, _ = range e.interfaces {
		// TODO Configure interface
		// TODO Move to namespace
	}
	return nil
}
