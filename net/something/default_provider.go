package something

import (
	"github.com/docker/docker/extensions"
	"strings"
)

type BuiltinProvider struct {
	BaseProvider
	hostMode      *BuiltInNetworkMode
	containerMode *BuiltInNetworkMode
}

func (d *BuiltinProvider) Init(s extensions.State) error {
	d.State = s
	d.hostMode = &BuiltInNetworkMode{
		hostMode: true,
		id:       "host",
	}
	d.containerMode = &BuiltInNetworkMode{
		hostMode: false,
		id:       "container",
	}
	return nil
}

func (d *BuiltinProvider) Shutdown(s extensions.State) error {
	return nil
}

func (d *BuiltinProvider) GetNetwork(idOrName string) (extensions.Network, error) {
	if idOrName == "host" {
		return d.hostMode, nil
	} else if strings.HasPrefix("container:", idOrName) {
		return d.containerMode, nil
	}

	return nil, nil
}

type BuiltInNetworkMode struct {
	id               string
	hostMode         bool
	defaultProvider  *BuiltinProvider
	namespaceFactory extensions.NamespaceFactory
}

func (d *BuiltInNetworkMode) Id() string {
	return d.id
}

func (d *BuiltInNetworkMode) CustomDnsSupported() bool {
	return d.hostMode
}

func (d *BuiltInNetworkMode) NamespaceFactory() extensions.NamespaceFactory {
	return d.namespaceFactory
}

func (d *BuiltInNetworkMode) Provider() extensions.NetworkProvider {
	return d.defaultProvider
}

func (d *BuiltInNetworkMode) ListEndpoints() ([]*extensions.Endpoint, error) {
	// TODO: need to pull from state
	return []*extensions.Endpoint{}, nil
}

func (d *BuiltInNetworkMode) LookupEndpoint(metaData map[string]string) (*extensions.Endpoint, error) {
	// TODO
	return nil, nil
}

func (d *BuiltInNetworkMode) CreateEndpoint(endpoint *extensions.Endpoint) error {
	return nil
}

func (d *BuiltInNetworkMode) ActivateEndpoint(endpoint *extensions.Endpoint) error {
	return nil
}

func (d *BuiltInNetworkMode) DeactivateEndpoint(endpoint *extensions.Endpoint) error {
	return nil
}

func (d *BuiltInNetworkMode) RemoveEndpoint(endpoint *extensions.Endpoint) error {
	return nil
}
