package daemon

import (
	"fmt"

	"github.com/docker/docker/engine"
	"github.com/docker/docker/graph"
	"github.com/docker/docker/pkg/parsers"
	"github.com/docker/docker/runconfig"
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
	// FIXME: this is necessary because daemon.Create might return a nil container
	// with a non-nil error. This should not happen! Once it's fixed we
	// can remove this workaround.
	if container != nil {
		job.Printf("%s\n", container.ID)
	}
	for _, warning := range buildWarnings {
		job.Errorf("%s\n", warning)
	}

	return engine.StatusOK
}

// Create creates a new container from the given configuration with a given name.
func (daemon *Daemon) Create(config *runconfig.Config, hostConfig *runconfig.HostConfig, name string) (*Container, []string, error) {
	var (
		warnings  []string
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
	if hostConfig != nil && config.SecurityOpt == nil {
		config.SecurityOpt, err = daemon.GenerateSecurityOpt(hostConfig.IpcMode)
		if err != nil {
			return nil, nil, err
		}
	}
	c := &Container{
		// FIXME: we should generate the ID here instead of receiving it as an argument
		ID:              utils.GenerateRandomID(),
		Created:         time.Now().UTC(),
		Path:            config.Entrypoint,
		Args:            args, //FIXME: de-duplicate from config
		Config:          config,
		hostConfig:      &runconfig.HostConfig{},
		Image:           img.ID, // Always use the resolved image id
		NetworkSettings: &NetworkSettings{},
		Driver:       daemon.driver.String(),
		ExecDriver:   daemon.execDriver.Name(),
		State:        NewState(),
		execCommands: newExecStore(),
	}
	// FIXME: find a clean home for this.
	if config.Hostname == "" {
		config.Hostname = c.ID[:12]
	}
	c.Path, c.Args = daemon.getEntrypointAndArgs(config.Entrypoint, config.Cmd)
	c.root = daemon.containerRoot(container.ID)

	// FIXME: move this into exec driver
	if err := parseSecurityOpt(c, config); err != nil {
		return nil, nil, err
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
	if err := daemon.execDriver.Init(c.ID); err != nil {
		return nil, nil, err
	}

	////////////////////////////
	//////////////////////////// BEGIN NEW NETWORKING HOOK
	// Here we allocate an endpoint for this container on the default network.
	// This replaces any consideration of "container name"
	////////////////////////////
	// By default join a network under the specified name

	netid, err := daemon.networks.Default()
	if err != nil {
		return nil, nil, err
	}
	defaultNet, err := daemon.networks.Get(netid)
	if err != nil {
		return nil, nil, err
	}

	// FIXME: we don't need the entire container root, just its namespace.
	// For this we need the persistent namespace patch from lk4d4 and icecrime.
	// (Otherwise the namespace is not created until the process is started),
	// and it is lost when the process terminates.
	// For now we assume the netns will be available at $ROOT/netns
	//
	// Note: it's ok if name is "". AddEndpoint is responsible for generating a
	// locally unique name in that case (ie "sad_einstein").
	//
	ep, ifaces, err := defaultNet.AddEndpoint(name, false)
	if err != nil {
		return nil, nil, err
	}
	for _, iface := range ifaces {
		// FIXME: execdriver must implement a new method AddIface
		if err := daemon.execdriver.AddIface(iface, container.ID); err != nil {
			return nil, nil, err
		}
	}

	// Expose ports on the new endpoint
	if container.Config.ExposedPorts != nil {
		for _, port := range config.ExposedPorts {
			if err := ep.Expose(port, false); err != nil {
				return nil, nil, err
			}
		}
	}

	// *Publish* particular ports as requested in HostConfig
	if container.hostConfig.PortBindings != nil {
		for p, b := range container.hostConfig.PortBindings {
			bindings[p] = []nat.PortBinding{}
			for _, bb := range b {
				if err := ep.Expose(p, true); err != nil {
					return nil, nil, err
				}
			}
		}
	}

	if err := container.ToDisk(); err != nil {
		return nil, nil, err
	}
	return container, warnings, nil
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
