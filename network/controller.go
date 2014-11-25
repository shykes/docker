package network

import (
	"sync"

	"github.com/docker/docker/core"
	"github.com/docker/docker/sandbox"
	"github.com/docker/docker/state"
)

type Controller struct {
	driver    Driver
	networks  map[core.DID]Network
	endpoints map[core.DID]Endpoint
	state     state.State
	mutex     sync.Mutex
}

func NewController(s state.State, driver Driver) (*Controller, error) {
	return &Controller{
		state:     s,
		driver:    driver,
		networks:  map[core.DID]Network{},
		endpoints: map[core.DID]Endpoint{},
	}, nil
}

func (c *Controller) ListNetworks() []core.DID {
	dids := []core.DID{}
	c.mutex.Lock()
	for did := range c.networks {
		dids = append(dids, did)
	}
	c.mutex.Unlock()

	return dids
}

func (c *Controller) GetNetwork(id core.DID) (Network, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.networks[id], nil
}

func (c *Controller) RemoveNetwork(id core.DID) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if err := c.driver.RemoveNetwork(string(id), c.state.Scope(id)); err != nil {
		return err
	}

	delete(c.networks, id)

	return nil
}

func (c *Controller) NewNetwork() (Network, error) {
	did := core.DID("") // core.GenerateDID() // func Generatecore.DID() core.DID { return core.DID(uuid.New()) }
	err := c.driver.AddNetwork(string(did), c.state.Scope(did))
	if err != nil {
		return nil, err
	}

	c.mutex.Lock()
	//c.networks[did] = net
	c.mutex.Unlock()

	return nil, nil
}

func (c *Controller) GetEndpoint(id core.DID) (Endpoint, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.endpoints[id], nil
}

// A network is a perimeter of IP connectivity between network services.
type Network interface {
	// Id returns the network's globally unique identifier
	Id() core.DID

	// List returns the IDs of available networks
	List() ([]core.DID, error)

	// Link makes the specified sandbox reachable as a named endpoint on the network.
	// If the endpoint already exists, the call will either fail (replace=false), or
	// unlink the previous endpoint.
	//
	// For example mynet.Link(mysandbox, "db", true) will make mysandbox available as
	// "db" on mynet, and will replace the other previous endpoint, if any.
	//
	// The same sandbox can be linked to multiple networks.
	// The same sandbox can be linked to the same network as multiple endpoints.
	Link(s sandbox.Sandbox, name string, replace bool) (Endpoint, error)

	// Unlink removes the specified endpoint, unlinking the corresponding sandbox from the
	// network.
	Unlink(name string) error
}

// An endpoint represents a particular member of a network, registered under a certain name
// and reachable over IP by other endpoints on the same network.
type Endpoint interface {
	Name() string
}
