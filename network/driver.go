package network

import (
	"github.com/docker/docker/sandbox"
	"github.com/docker/docker/state"
)

/*
  Driver describes the interface for creating network drivers.

  All functions are covered in network/controller.go
*/
type Driver interface {
	Restore(netstate state.State) error
	AddNetwork(netid string, params []string) error
	RemoveNetwork(netid string) error
	GetNetwork(id string) (Network, error)

	Link(netid, name string, sb sandbox.Sandbox, replace bool) (Endpoint, error)
	Unlink(netid, name string, sb sandbox.Sandbox) error
}
