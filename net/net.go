package net

import (
	"errors"
	e "github.com/docker/docker/engine"
	"github.com/docker/docker/extensions"
)

var NetworkingThingy *Networking

type Networking struct {
	Providers []extensions.NetworkProvider
	state     extensions.State
}

//func NewNetworking() (*Networking, error) {
//return &Networking{
//}, nil
//}

func (n *Networking) CreateContainerEndpoint(networkName string, containerId string) (extensions.Network, *extensions.Endpoint, error) {
	network, err := n.GetNetwork(networkName)
	if err != nil {
		return nil, nil, err
	}

	if network == nil {
		return nil, nil, errors.New("Failed to find network " + networkName)
	}

	endpoint := &extensions.Endpoint{
		MetaData: map[string]string{
			extensions.CONTAINER_ID: containerId,
		},
	}

	err = network.CreateEndpoint(endpoint)

	return network, endpoint, err
}

func containerMetaData(containerId string) map[string]string {
	return map[string]string{
		extensions.CONTAINER_ID: containerId,
	}
}

func (n *Networking) RemoveContainerEndpoint(networkName string, containerId string) error {
	network, err := n.GetNetwork(networkName)
	if err != nil {
		return err
	}

	if network == nil {
		return nil
	}

	endpoint, err := network.LookupEndpoint(containerMetaData(containerId))
	if err != nil {
		return err
	}

	if endpoint == nil {
		return nil
	}

	return network.RemoveEndpoint(endpoint)
}

func (n *Networking) GetNetwork(name string) (network extensions.Network, err error) {
	for _, provider := range n.Providers {
		network, err := provider.NetworkExtension().GetNetwork(name)

		// Ignore errors if we can find the right network somewhere else
		if err != nil {
			continue
		}

		if network != nil {
			return network, err
		}
	}

	if err != nil {
		return nil, err
	}

	return nil, errors.New("Failed to find network for " + name)
}

func (n *Networking) Install(eng *e.Engine) error {
	eng.Register("net_create", n.CmdCreate)
	eng.Register("net_rm", n.CmdRm)
	eng.Register("net_ls", n.CmdLs)
	eng.Register("net_join", n.CmdJoin)
	eng.Register("net_leave", n.CmdLeave)
	eng.Register("net_import", n.CmdImport)
	eng.Register("net_export", n.CmdExport)
	return nil
}

func (n *Networking) CmdCreate(j *e.Job) e.Status {
	if len(j.Args) != 1 {
		return j.Errorf("usage: %s NAME", j.Name)
	}
	// FIXME
	return e.StatusOK
}

func (n *Networking) CmdLs(j *e.Job) e.Status {
	if len(j.Args) != 1 {
		return j.Errorf("usage: %s NAME", j.Name)
	}
	// FIXME
	return e.StatusOK
}

func (n *Networking) CmdRm(j *e.Job) e.Status {
	if len(j.Args) != 1 {
		return j.Errorf("usage: %s NAME", j.Name)
	}
	// FIXME
	return e.StatusOK
}

func (n *Networking) CmdJoin(j *e.Job) e.Status {
	if len(j.Args) != 1 {
		return j.Errorf("usage: %s NAME", j.Name)
	}
	// FIXME
	return e.StatusOK
}

func (n *Networking) CmdLeave(j *e.Job) e.Status {
	if len(j.Args) != 1 {
		return j.Errorf("usage: %s NAME", j.Name)
	}
	// FIXME
	return e.StatusOK
}

func (n *Networking) CmdImport(j *e.Job) e.Status {
	if len(j.Args) != 1 {
		return j.Errorf("usage: %s NAME", j.Name)
	}
	// FIXME
	return e.StatusOK
}

func (n *Networking) CmdExport(j *e.Job) e.Status {
	if len(j.Args) != 1 {
		return j.Errorf("usage: %s NAME", j.Name)
	}
	// FIXME
	return e.StatusOK
}
