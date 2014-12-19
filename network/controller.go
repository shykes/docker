package network

import (
	"fmt"
	"sync"

	"github.com/docker/docker/sandbox"
	"github.com/docker/docker/state"
)

type Controller struct {
	// FIXME:networking This probably should be a driver list
	driver    Driver
	networks  map[string]Network
	endpoints map[string]Endpoint
	state     state.State
	mutex     sync.Mutex
	// Containers at creation time will create an Endpoint on the default
	// network identified by this ID.
	DefaultNetworkID string
}

func NewController(s state.State) *Controller {
	return &Controller{
		state:     s,
		networks:  map[string]Network{},
		endpoints: map[string]Endpoint{},
	}
}

// FIXME:networking
func (c *Controller) AddDriver(driver Driver, name string) error {
	c.driver = driver
	return nil
}

// FIXME:networking
func (c *Controller) HasDriver() bool {
	return c.driver != nil
}

func (c *Controller) Restore(s state.State) error {
	// FIXME:networking not yet implemented

	// Load default network, creating one if it doesn't exist.
	/*
		defaultNetSettings, err := s.GetObj("/default")
		if os.IsNotExist(err) { // no default network created
			defaultNet, err := c.NewNetwork()
			if err != nil {
				return err
			}
			s.Set("/default/id", defaultNet.Id)
			if err := s.Commit(); err != nil {

			}
		}
		// Set DefaultNetworkId
	*/

	// Load list of networks
	// Call drivers.Restore

	return c.driver.Restore(s)
}

func (c *Controller) ListNetworks() []string {
	dids := []string{}
	c.mutex.Lock()
	for did := range c.networks {
		dids = append(dids, did)
	}
	c.mutex.Unlock()

	return dids
}

func (c *Controller) GetNetwork(id string) (Network, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	netw, ok := c.networks[id]
	if !ok {
		return nil, fmt.Errorf("unknown network %q", id)
	}
	return netw, nil
}

func (c *Controller) RemoveNetwork(id string) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if err := c.driver.RemoveNetwork(string(id)); err != nil {
		return err
	}

	delete(c.networks, id)

	return nil
}

func (c *Controller) NewNetwork(name string, args []string) (Network, error) {
	if err := c.driver.AddNetwork(name, args); err != nil {
		return nil, err
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()
	thisNet, err := c.driver.GetNetwork(name)
	if err != nil {
		return nil, err
	}
	c.networks[name] = thisNet

	return thisNet, nil
}

func (c *Controller) GetEndpoint(id string) (Endpoint, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.endpoints[id], nil
}

// A network is a perimeter of IP connectivity between network services.
type Network interface {
	// Id returns the network's globally unique identifier
	Id() string

	// List returns the IDs of available networks
	List() []string

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

	// FIXME:networking Should we take nat.Port string format (i.e.: "80/tcp")?
	Expose(portspec string, publish bool) error
	Network() Network
}
