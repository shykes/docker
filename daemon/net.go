package daemon

import (
	"github.com/docker/docker/engine"
	"github.com/docker/docker/network"
)

func (d *Daemon) CmdNetCreate(job *engine.Job) engine.Status {
	if len(job.Args) < 1 {
		return job.Errorf("usage: %s NAME", job.Name)
	}

	params := []string{}

	if len(job.Args) > 1 {
		params = job.Args[1:]
	}

	thisNet, err := d.networks.NewNetwork(job.Args[0], params)
	if err != nil {
		return job.Error(err)
	}

	job.Printf("%v\n", thisNet.Id())
	return engine.StatusOK
}

func (d *Daemon) CmdNetLs(job *engine.Job) engine.Status {
	netw := d.networks.ListNetworks()

	table := engine.NewTable("Name", len(netw))
	for _, netid := range netw {
		item := &engine.Env{}
		item.Set("Name", netid)
		table.Add(item)
	}

	if _, err := table.WriteTo(job.Stdout); err != nil {
		return job.Error(err)
	}
	return engine.StatusOK
}

func (d *Daemon) CmdNetRm(job *engine.Job) engine.Status {
	if len(job.Args) != 1 {
		return job.Errorf("usage: %s NAME", job.Name)
	}

	if err := d.networks.RemoveNetwork(job.Args[0]); err != nil {
		return job.Error(err)
	}
	return engine.StatusOK
}

func (d *Daemon) CmdNetJoin(job *engine.Job) engine.Status {
	if len(job.Args) != 3 {
		return job.Errorf("usage: %s NETWORK CONTAINER NAME", job.Name)
	}

	networkID := job.Args[0]
	net, err := d.networks.GetNetwork(networkID)
	if err != nil {
		return job.Error(err)
	}

	// FIXME The provided CONTAINER could be the 'user facing ID'. but not
	// necessarily the sandbox ID itself: we're keeping things simple here.
	containerID := job.Args[1]
	if fullid, err := d.idIndex.Get(containerID); err == nil {
		containerID = fullid
	}

	sandbox, err := d.sandboxes.Get(containerID)
	if err != nil {
		return job.Error(err)
	}

	ep, err := net.Link(sandbox, job.Args[2], false)
	if err != nil {
		return job.Error(err)
	}

	// FIXME Provides output for `docker ps`
	if c := d.containers.Get(containerID); c != nil {
		c.Endpoints[networkID] = append(c.Endpoints[networkID], ep.Name())
	}
	return engine.StatusOK
}

func (d *Daemon) CmdNetLeave(job *engine.Job) engine.Status {
	if len(job.Args) != 2 {
		return job.Errorf("usage: %s NETWORK NAME", job.Name)
	}

	net, err := d.networks.GetNetwork(job.Args[0])
	if err != nil {
		return job.Error(err)
	}

	// FIXME: Network.Unlink should give access to the sandbox, so that the
	// driver can do cleanup.
	if err := net.Unlink(job.Args[1]); err != nil {
		return job.Error(err)
	}
	return engine.StatusOK
}

func (d *Daemon) CmdNetImport(job *engine.Job) engine.Status {
	if len(job.Args) != 1 {
		return job.Errorf("usage: %s NAME", job.Name)
	}
	// FIXME
	return engine.StatusOK
}

func (d *Daemon) CmdNetExport(job *engine.Job) engine.Status {
	if len(job.Args) != 1 {
		return job.Errorf("usage: %s NAME", job.Name)
	}
	// FIXME
	return engine.StatusOK
}

func (d *Daemon) RegisterNetworkDriver(driver network.Driver, name string) error {
	return d.networks.AddDriver(driver, name)
}

func (d *Daemon) UnregisterNetworkDriver(name string) error {
	// FIXME Forward to d.networks
	return nil
}
