package simplebridge

import (
	"flag"
	"fmt"
	"sync"

	"github.com/docker/docker/network"
	"github.com/docker/docker/sandbox"
	"github.com/docker/docker/state"
)

const (
	maxVethName      = 10
	maxVethSuffixLen = 2
	maxVethSuffix    = 99
)

type BridgeDriver struct {
	schema *Schema
	state  state.State
	mutex  sync.Mutex
}

func (d *BridgeDriver) GetNetwork(network string) (network.Network, error) {
	return d.loadNetwork(network)
}

func (d *BridgeDriver) Restore(s state.State) error {
	d.state = s
	return nil
}

// discovery driver? should it be hooked here or in the core?
func (d *BridgeDriver) Link(network, endpoint string, s sandbox.Sandbox, replace bool) (network.Endpoint, error) {
	if len(endpoint) > maxVethName {
		return nil, fmt.Errorf("endpoint %q is too long, must be %d characters", endpoint, maxVethName)
	}

	d.mutex.Lock()
	defer d.mutex.Unlock()

	netObj, err := d.loadNetwork(network)
	if err != nil {
		return nil, err
	}

	ep := &BridgeEndpoint{
		network: netObj,
		ID:      endpoint,
	}

	if ep, err := d.loadEndpoint(network, endpoint); ep != nil && err != nil && !replace {
		return nil, fmt.Errorf("Endpoint %q already taken", endpoint)
	}

	if err := d.schema.Endpoint(network, endpoint).Create(""); err != nil {
		return nil, fmt.Errorf("Trouble creating endpoint %q for network %q: %v", endpoint, network, err)
	}

	if err := ep.configure(endpoint, s); err != nil {
		return nil, fmt.Errorf("Trouble configuring endpoint %q for network %q: %v", endpoint, network, err)
	}

	if err := d.saveEndpoint(network, ep); err != nil {
		return nil, err
	}

	return ep, nil
}

func (d *BridgeDriver) Unlink(netid, name string, sb sandbox.Sandbox) error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	ep, err := d.loadEndpoint(netid, name)
	if err != nil {
		return fmt.Errorf("No endpoint for name %q: %v", name, err)
	}

	if err := ep.deconfigure(name); err != nil {
		return fmt.Errorf("Trouble deconfiguring endpoint %q for network %q: %v", name, netid, err)
	}

	if err := d.schema.Endpoint(netid, name).Remove(""); err != nil {
		return fmt.Errorf("Trouble removing endpoint %q for network %q: %v", name, netid, err)
	}

	return nil
}

func (d *BridgeDriver) AddNetwork(network string, args []string) error {
	// FIXME this should be abstracted from the network driver

	fs := flag.NewFlagSet("simplebridge", flag.ContinueOnError)
	// FIXME need to figure out a way to prop usage
	fs.Usage = func() {}
	peer := fs.String("peer", "", "VXLan peer to contact")
	vlanid := fs.Uint("vid", 42, "VXLan VLAN ID")
	port := fs.Uint("port", 4789, "VXLan Tunneling Port")
	device := fs.String("dev", "eth0", "Device to set as the vxlan endpoint")

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("Trouble parsing argument for AddNetwork on network %q: %v", network, err)
	}

	if err := d.schema.Network(network).Create(""); err != nil {
		return fmt.Errorf("Trouble creating network %q: %v", network, err)
	}

	bridge, err := d.createBridge(network, *vlanid, *port, *peer, *device)
	if err != nil {
		return err
	}

	if err := d.saveNetwork(network, bridge); err != nil {
		return err
	}

	return nil
}

func (d *BridgeDriver) RemoveNetwork(network string) error {
	bridge, err := d.loadNetwork(network)
	if err != nil {
		return err
	}

	if err := d.schema.Network(network).Remove(""); err != nil {
		return fmt.Errorf("Trouble removing network %q: %v", network, err)
	}

	return bridge.destroy()
}
