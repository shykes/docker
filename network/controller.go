/*
  network is a package that describes several interfaces for network drivers.

  Please see extensions/ for documentation on how this package should be implemented.

  Basic terminology:

  * Driver: a piece of code used for managing networks and endpoints.
  * Network: a full network intended to be used by many containers. In
    simplebridge this is a single bridge interface.
  * Endpoint: An interface for a container. Containers may have multiple
    interfaces.
*/
package network

import (
	"fmt"
	"sync"

	"github.com/docker/docker/sandbox"
	"github.com/docker/docker/state"
)

/*
  A controller is a singleton which lives inside daemon/ and controls the state
  and organization of network objects.
*/
type Controller struct {
	driver           Driver              // Driver that powers the creation of networks and endpoints.
	networks         map[string]Network  // network objects, mapped from network name.
	endpoints        map[string]Endpoint // endpoint objects, mapped from network name.
	state            state.State         // see `state/` for an interface which describes state.
	mutex            sync.Mutex          // Lock for creating Networks and Endpoints. May be removed.
	DefaultNetworkID string              // Containers at creation time will create an Endpoint on the default network identified by this ID.
}

// Create a new controller. Should only be used by `daemon/`.
func NewController(s state.State) *Controller {
	return &Controller{
		state:     s,
		networks:  map[string]Network{},
		endpoints: map[string]Endpoint{},
	}
}

// Add the driver to the controller for use.
func (c *Controller) AddDriver(driver Driver, name string) error {
	c.driver = driver
	return nil
}

// Predictate to determine whether or not we currently have an associated
// driver.
func (c *Controller) HasDriver() bool {
	return c.driver != nil
}

// Restore takes a state object and "replays" the state to the driver. This way
// the driver can refresh its representation of the docker networking system
// and create or register missing devices if necessary.
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

// List all registered networks
func (c *Controller) ListNetworks() []string {
	dids := []string{}
	c.mutex.Lock()
	for did := range c.networks {
		dids = append(dids, did)
	}
	c.mutex.Unlock()

	return dids
}

// Get a network by name.
func (c *Controller) GetNetwork(id string) (Network, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	netw, ok := c.networks[id]
	if !ok {
		return nil, fmt.Errorf("unknown network %q", id)
	}
	return netw, nil
}

// Remove a network by name.
func (c *Controller) RemoveNetwork(id string) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if err := c.driver.RemoveNetwork(string(id)); err != nil {
		return err
	}

	delete(c.networks, id)

	return nil
}

// Create a new network. Expects a network name and a string array of flags to
// pass to the driver.
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
	Name() string // name of the endpoint
	// FIXME:networking Should we take nat.Port string format (i.e.: "80/tcp")?
	Expose(portspec string, publish bool) error // expose a port from the host to the endpoint
	Network() Network                           // network used to create this endpoint.
}
