package net

import (
	"fmt"

	"github.com/docker/docker/engine"
	"github.com/docker/docker/extensions"
	"github.com/docker/docker/utils"

	log "github.com/Sirupsen/logrus"
)

// The network.Networks is responsible for orchestrating networking extensions
// and managing the lifetime of networks and endpoints.
func New(state extensions.State) *Networks {
	service := &Networks{
		state:      state,
		extensions: make(map[string]extensions.NetExtension),
		networks:   make(map[string]*Network),
	}

	// As a first implementation of extension system. networks extensions are
	// compiled in, but we reference them through symbolic names nevertheless
	// to fit the target behavior.
	service.loadExtension("debug", MockExtension(log.New()))
	//service.loadExtension("simplebridge", nil)
	return service
}

type Networks struct {
	state      extensions.State
	extensions map[string]extensions.NetExtension
	networks   map[string]*Network
}

// Install service specific job handlers on the daemon engine.
func (s *Networks) Install(eng *engine.Engine) error {
	for name, handler := range map[string]engine.Handler{
		"net_create": s.networkCreate,
		"net_export": s.networkExport,
		"net_import": s.networkImport,
		"net_join":   s.networkJoin,
		"net_ls":     s.networkList,
		"net_leave":  s.networkLeave,
		"net_rm":     s.networkRemove,
	} {
		if err := eng.Register(name, handler); err != nil {
			return fmt.Errorf("failed to register %q: %v\n", name, err)
		}
	}
	return nil
}

// Retrieve the name of the default network
func (s *Networks) Default() string {
	// TODO
	for k, _ := range s.networks {
		return k
	}
	return "DEFAULT NETWORK ID"
}

// Retrieve the previously created network identified by netid.
func (s *Networks) Get(netid string) (*Network, error) {
	if n, ok := s.networks[netid]; ok {
		return n, nil
	}
	return nil, fmt.Errorf("unknown network %q", netid)
}

// Create a new network using the specified extension name. The extensions must
// already be loaded for the call to succeed.
func (s *Networks) networkCreate(job *engine.Job) engine.Status {
	// TODO Extension should not be part of the message
	extName := job.Getenv("extension")
	if err := s.addNetwork(extName, map[string]string{}); err != nil {
		return job.Error(err)
	}
	return engine.StatusOK
}

func (s *Networks) networkExport(job *engine.Job) engine.Status {
	return engine.StatusOK
}

func (s *Networks) networkImport(job *engine.Job) engine.Status {
	return engine.StatusOK
}

func (s *Networks) networkJoin(job *engine.Job) engine.Status {
	return engine.StatusOK
}

func (s *Networks) networkLeave(job *engine.Job) engine.Status {
	return engine.StatusOK
}

func (s *Networks) networkList(job *engine.Job) engine.Status {
	// TODO Iterate extensions
	// TODO Iterate networks inside extensions
	return engine.StatusOK
}

func (s *Networks) networkRemove(job *engine.Job) engine.Status {
	return engine.StatusErr
}

func (s *Networks) addNetwork(extName string, settings map[string]string) error {
	extension := s.extensionByName(extName)
	if extension == nil {
		return fmt.Errorf("Unknown network extensions '%s'", extName)
	}

	id := utils.GenerateRandomID()
	if err := extension.AddNet(id, s.state); err != nil {
		return err
	}

	s.networks[id] = NewNetwork(id, extension, s.state)
	return nil
}

func (s *Networks) loadExtension(name string, extension extensions.NetExtension) error {
	if _, ok := s.extensions[name]; ok {
		return fmt.Errorf("Network extension '%s' is already registered", name)
	}

	// Initialize the extension
	if err := extension.Init(s.state); err != nil {
		return fmt.Errorf("Failed to initialize network extension '%s': %v", name, err)
	}
	s.extensions[name] = extension

	// TODO Restore previous state (if any)
	// We attempt to create a default network (meaning: parameterless creation)
	// for extensions which have no known previous state. Failure is not an
	// error as a driver may very well have required parameters for creation.
	if err := s.addNetwork(name, map[string]string{}); err != nil {
		log.Debugf("Failed to create default network for extension '%s': %v", name, err)
	}
	return nil
}

func (s *Networks) extensionByName(name string) extensions.NetExtension {
	if ext, ok := s.extensions[name]; ok {
		return ext
	}
	return nil
}

func (s *Networks) networkByName(name string) *Network {
	if net, ok := s.networks[name]; ok {
		return net
	}
	return nil
}
