package net

import (
	"fmt"

	e "github.com/docker/docker/engine"
)

type Networks struct {
	//FIXME
	getContainer   func(string) (Container, error)
	nets           map[string]*Network
	defaultNetwork string
}

func New(getContainer func(string) (Container, error), root string) (*Networks, error) {
	// FIXME: instead of passing root, daemon could pass a pre-allocated
	// State. Then other subsystems could start using State... :)
	// One step at a time.
	return &Networks{
		getContainer: getContainer,
		nets:         make(map[string]*Network),
	}, nil
}

func (n *Networks) SetDefault(netid string) {
	n.defaultNetwork = netid
}

func (n *Networks) Default() string {
	return n.defaultNetwork
}

func (n *Networks) Get(netid string) (*Network, error) {
	// FIXME
	net, ok := n.nets[netid]
	if !ok {
		return nil, fmt.Errorf("No such network: %s", netid)
	}
	return net, nil
}

func (n *Networks) Set(netid string, net *Network) error {
	if _, exists := n.nets[netid]; exists {
		return fmt.Errorf("Network name %#v is already taken", netid)
	}

	n.nets[netid] = net
	return nil
}

func (n *Networks) Delete(netid string) error {
	if _, exists := n.nets[netid]; !exists {
		return fmt.Errorf("No such network: %#v", netid)
	}

	delete(n.nets, netid)
	return nil
}

type Network struct {
	endpoints map[string]*Endpoint
	services  map[string]*Service
}

type Container interface {
	NSPath() string
	PortSet
}

func NewNetwork() *Network {
	return &Network{
		endpoints: make(map[string]*Endpoint),
		services:  make(map[string]*Service),
	}
}

func (n *Network) AddEndpoint(c Container, name string, replace bool) (*Endpoint, error) {
	if name == "" {
		// FIXME: generate and reserve a random name
	}

	if _, exists := n.endpoints[name]; exists {
		if !replace {
			return nil, fmt.Errorf("Endpoint name %#v is already taken", name)
		}
	}

	ep := &Endpoint{
		name: name,
		c:    c,
	}
	// FIXME: here, go over extensions, call AddEndpoint, place interfaces
	// in ns, apply configuration, etc.
	// Perhaps this could be abstracted by execdriver, but we can worry about that
	// later.
	n.endpoints[name] = ep
	return ep, nil
}

func (n *Network) RemoveEndpoint(name string) error {
	if _, exists := n.endpoints[name]; !exists {
		return fmt.Errorf("No such endpoint: %#v", name)
	}

	delete(n.endpoints, name)
	return nil
}

type Endpoint struct {
	name string
	addr []IP
	c    Container
	// FIXME: per-endpoint port filtering as an advanced feature?
}

type IP string

type Service struct {
	name    string
	backend *Endpoint
	proto   string // "tcp" or "udp"
	port    uint16
}

type PortSet interface {
	// FIXME
	// This holds a set of ports within the universe of tcp and udp
	// ports

	// This is similar to daemon/networkdriver/portallocator/protoMap
	// but without the baggage.
}

func (n *Networks) Install(eng *e.Engine) error {
	eng.Register("net_create", n.CmdCreate)
	eng.Register("net_rm", n.CmdRm)
	eng.Register("net_ls", n.CmdLs)
	eng.Register("net_join", n.CmdJoin)
	eng.Register("net_leave", n.CmdLeave)
	eng.Register("net_import", n.CmdImport)
	eng.Register("net_export", n.CmdExport)
	return nil
}

func (n *Networks) CmdCreate(j *e.Job) e.Status {
	if len(j.Args) != 1 {
		return j.Errorf("usage: %s NAME", j.Name)
	}

	net := NewNetwork()
	if err := n.Set(j.Args[0], net); err != nil {
		return j.Error(err)
	}

	return e.StatusOK
}

func (n *Networks) CmdLs(j *e.Job) e.Status {
	networks := e.NewTable("Name", len(n.nets))

	for name, _ := range n.nets {
		network := &e.Env{}
		network.Set("Name", name)
		networks.Add(network)
	}

	if _, err := networks.WriteTo(j.Stdout); err != nil {
		return j.Error(err)
	}

	return e.StatusOK
}

func (n *Networks) CmdRm(j *e.Job) e.Status {
	if len(j.Args) != 1 {
		return j.Errorf("usage: %s NAME", j.Name)
	}

	if err := n.Delete(j.Args[0]); err != nil {
		return j.Error(err)
	}

	return e.StatusOK
}

func (n *Networks) CmdJoin(j *e.Job) e.Status {
	if len(j.Args) != 3 {
		return j.Errorf("usage: %s NETWORK CONTAINER NAME", j.Name)
	}

	net, err := n.Get(j.Args[0])
	if err != nil {
		return j.Error(err)
	}

	container, err := n.getContainer(j.Args[1])
	if err != nil {
		return j.Error(err)
	}

	if _, err := net.AddEndpoint(container, j.Args[2], false); err != nil {
		return j.Error(err)
	}

	return e.StatusOK
}

func (n *Networks) CmdLeave(j *e.Job) e.Status {
	if len(j.Args) != 2 {
		return j.Errorf("usage: %s NETWORK NAME", j.Name)
	}

	net, err := n.Get(j.Args[0])
	if err != nil {
		return j.Error(err)
	}

	if err := net.RemoveEndpoint(j.Args[1]); err != nil {
		return j.Error(err)
	}

	return e.StatusOK
}

func (n *Networks) CmdImport(j *e.Job) e.Status {
	if len(j.Args) != 1 {
		return j.Errorf("usage: %s NAME", j.Name)
	}
	// FIXME
	return e.StatusOK
}

func (n *Networks) CmdExport(j *e.Job) e.Status {
	if len(j.Args) != 1 {
		return j.Errorf("usage: %s NAME", j.Name)
	}
	// FIXME
	return e.StatusOK
}
