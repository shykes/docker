package daemon

import (
	"fmt"
	"time"

	"github.com/docker/docker/engine"
	"github.com/docker/docker/graph"
	"github.com/docker/docker/network"
	"github.com/docker/docker/pkg/parsers"
	"github.com/docker/docker/runconfig"
	"github.com/docker/docker/utils"
	"github.com/docker/libcontainer/label"
)

func (daemon *Daemon) ContainerCreate(job *engine.Job) engine.Status {
	var name string
	if len(job.Args) == 1 {
		name = job.Args[0]
	} else if len(job.Args) > 1 {
		return job.Errorf("Usage: %s", job.Name)
	}

	config := runconfig.ContainerConfigFromJob(job)
	if config.Memory != 0 && config.Memory < 4194304 {
		return job.Errorf("Minimum memory limit allowed is 4MB")
	}
	if config.Memory > 0 && !daemon.SystemConfig().MemoryLimit {
		job.Errorf("Your kernel does not support memory limit capabilities. Limitation discarded.\n")
		config.Memory = 0
	}
	if config.Memory > 0 && !daemon.SystemConfig().SwapLimit {
		job.Errorf("Your kernel does not support swap limit capabilities. Limitation discarded.\n")
		config.MemorySwap = -1
	}

	var hostConfig *runconfig.HostConfig
	if job.EnvExists("HostConfig") {
		hostConfig = runconfig.ContainerHostConfigFromJob(job)
	} else {
		// Older versions of the API don't provide a HostConfig.
		hostConfig = nil
	}

	container, buildWarnings, err := daemon.Create(config, hostConfig, name)
	if err != nil {
		if daemon.Graph().IsNotExist(err) {
			_, tag := parsers.ParseRepositoryTag(config.Image)
			if tag == "" {
				tag = graph.DEFAULTTAG
			}
			return job.Errorf("No such image: %s (tag: %s)", config.Image, tag)
		}
		return job.Error(err)
	}
	if !container.Config.NetworkDisabled && daemon.SystemConfig().IPv4ForwardingDisabled {
		job.Errorf("IPv4 forwarding is disabled.\n")
	}
	container.LogEvent("create")

	job.Printf("%s\n", container.ID)

	for _, warning := range buildWarnings {
		job.Errorf("%s\n", warning)
	}

	return engine.StatusOK
}

func (daemon *Daemon) attachContainerToDefaultNetwork(cid, name string) (network.Endpoint, error) {
	// Retrieve the default network that the new container should be joining.
	netid := daemon.networks.DefaultNetworkID
	defaultNet, err := daemon.networks.GetNetwork(netid)
	if err != nil {
		return nil, err
	}

	// Retrieve the Sandbox corresponding to the starting container.
	sandbox, err := daemon.sandboxes.Get(cid)
	if err != nil {
		return nil, err
	}

	// We need a name for the default endpoint.
	if name == "" {
		if name, err = daemon.generateNewName(cid); err != nil {
			return nil, err
		}

		// FIXME:networking Ugly hack: remove the leading '/', cap length.
		name = name[1:6]
	}

	// Link the sandbox to the network, thus creating a new endpoint with the
	// provided name.
	// FIXME:networking Do we need Link() to return the Endpoint?
	// FIXME:networking Deal with link name length
	ep, err := defaultNet.Link(sandbox, name, false /* replace */)
	if err != nil {
		return nil, err
	}

	return ep, nil
}

// Create creates a new container from the given configuration with a given name.
func (daemon *Daemon) Create(config *runconfig.Config, hostConfig *runconfig.HostConfig, name string) (*Container, []string, error) {
	var (
		warnings []string
	)

	// FIXME: installing images should be done out of band.
	img, err := daemon.repositories.LookupImage(config.Image)
	if err != nil {
		return nil, nil, err
	}
	if err := img.CheckDepth(); err != nil {
		return nil, nil, err
	}
	if warnings, err = daemon.mergeAndVerifyConfig(config, img); err != nil {
		return nil, nil, err
	}
	if hostConfig != nil && hostConfig.SecurityOpt == nil {
		hostConfig.SecurityOpt, err = daemon.GenerateSecurityOpt(hostConfig.IpcMode)
		if err != nil {
			return nil, nil, err
		}
	}
	c := &Container{
		// FIXME: we should generate the ID here instead of receiving it as an argument
		ID:              utils.GenerateRandomID(),
		Created:         time.Now().UTC(),
		Config:          config,
		hostConfig:      &runconfig.HostConfig{},
		Image:           img.ID, // Always use the resolved image id
		NetworkSettings: &NetworkSettings{},
		Driver:          daemon.driver.String(),
		ExecDriver:      daemon.execDriver.Name(),
		State:           NewState(),
		execCommands:    newExecStore(),
	}
	// FIXME: find a clean home for this.
	if config.Hostname == "" {
		config.Hostname = c.ID[:12]
	}
	c.Path, c.Args = daemon.getEntrypointAndArgs(config.Entrypoint, config.Cmd)
	c.root = daemon.containerRoot(c.ID)

	// FIXME: move this into exec driver
	if hostConfig != nil {
		if err := parseSecurityOpt(c, hostConfig); err != nil {
			return nil, nil, err
		}
	}

	// FIXME: Register relies on the concept of a single container name.
	// We are deprecating this concept (only network endpoint have names now).
	// CONCLUSION -> Register must stop dealing with names
	if err := daemon.Register(c); err != nil {
		return nil, nil, err
	}
	if err := daemon.createRootfs(c, img); err != nil {
		return nil, nil, err
	}

	// Initialize sandboxing environment (ie actual kernel namespaces etc.)
	if err := daemon.execDriver.Init(c.ID, map[string]string{"net": ""}); err != nil {
		return nil, nil, err
	}

	// By default join a network under the specified name
	ep, err := daemon.attachContainerToDefaultNetwork(c.ID, name)
	if err != nil {
		return nil, nil, err
	}

	// Expose ports on the new endpoint
	if c.Config.ExposedPorts != nil {
		for port, _ := range config.ExposedPorts {
			if err := ep.Expose(port.String(), false); err != nil {
				return nil, nil, err
			}
		}
	}

	// *Publish* particular ports as requested in HostConfig
	if c.hostConfig.PortBindings != nil {
		for p, b := range c.hostConfig.PortBindings {
			if err := ep.Expose(p.String(), true); err != nil {
				return nil, nil, err
			}

			// FIXME:networking Port is published at the network driver level,
			// but we are missing the host behaviour: should we rely on
			// daemon/networkdriver/portmapper/ in a first version?
			for _, bb := range b {
				_ = bb
			}
		}
	}

	if err := c.Mount(); err != nil {
		return nil, nil, err
	}
	defer c.Unmount()
	if err := c.prepareVolumes(); err != nil {
		return nil, nil, err
	}
	if err := c.ToDisk(); err != nil {
		return nil, nil, err
	}
	return c, warnings, nil
}

func (daemon *Daemon) GenerateSecurityOpt(ipcMode runconfig.IpcMode) ([]string, error) {
	if ipcMode.IsHost() {
		return label.DisableSecOpt(), nil
	}
	if ipcContainer := ipcMode.Container(); ipcContainer != "" {
		c := daemon.Get(ipcContainer)
		if c == nil {
			return nil, fmt.Errorf("no such container to join IPC: %s", ipcContainer)
		}
		if !c.IsRunning() {
			return nil, fmt.Errorf("cannot join IPC of a non running container: %s", ipcContainer)
		}

		return label.DupSecOpt(c.ProcessLabel), nil
	}
	return nil, nil
}
