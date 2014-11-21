package net

import (
	e "github.com/docker/docker/engine"
)

type Networks struct {
	//FIXME
	nets map[string]*Network
}

func New(root string) (*Networks, error) {
	// FIXME: instead of passing root, daemon could pass a pre-allocated
	// State. Then other subsystems could start using State... :)
	// One step at a time.
	return &Networks{
		nets: make(map[string]*Network),
	}, nil
}

func (n *Networks) Default() string {
	// FIXME
	return "THE PLUMBING IS NOT YET IN PLACE TO SELECT A DEFAULT NETWORK"
}

func (n *Networks) Get(netid string) (*Network, error) {
	// FIXME
	net, ok := n.nets[netid]
	if ok != nil {
		return nil, os.ErrNotExist{}
	}
	return net, nil
}

type Network struct {
	endpoints map[string]*Endpoint
	services  map[string]*Service
}

type Container interface {
	NSPath() string
	PortSet
}

func (n *Network) AddEndpoint(c Container, name string, replace bool) (*Endpoint, error) {
	if name == "" {
		// FIXME: generate and reserve a random name
	}
	// FIXME: check for name conflict, look at <replace> to determine behavior.
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

type Endpoint struct {
	name string
	addr []net.IP
	c    Container
	// FIXME: per-endpoint port filtering as an advanced feature?
}

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

func (n *Networks) Install(eng e.Engine) error {
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
	// FIXME
	return e.StatusOK
}

func (n *Networks) CmdLs(j *e.Job) e.Status {
	if len(j.Args) != 1 {
		return j.Errorf("usage: %s NAME", j.Name)
	}
	// FIXME
	return e.StatusOK
}

func (n *Networks) CmdRm(j *e.Job) e.Status {
	if len(j.Args) != 1 {
		return j.Errorf("usage: %s NAME", j.Name)
	}
	// FIXME
	return e.StatusOK
}

func (n *Networks) CmdJoin(j *e.Job) e.Status {
	if len(j.Args) != 1 {
		return j.Errorf("usage: %s NAME", j.Name)
	}
	// FIXME
	return e.StatusOK
}

func (n *Networks) CmdLeave(j *e.Job) e.Status {
	if len(j.Args) != 1 {
		return j.Errorf("usage: %s NAME", j.Name)
	}
	// FIXME
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
