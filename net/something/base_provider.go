package something

import (
	"github.com/docker/docker/extensions"
)

type BaseProvider struct {
	name             string
	namespaceFactory extensions.NamespaceFactory
	namingExtension  extensions.NamingExtension
	networkExtension extensions.NetworkExtension
	portExtension    extensions.PortExtension
	State            extensions.State
}

func (b *BaseProvider) Name() string {
	return b.name
}

func (b *BaseProvider) NamingExtension() extensions.NamingExtension {
	return b.namingExtension
}

func (b *BaseProvider) NamespaceFactory() extensions.NamespaceFactory {
	return b.namespaceFactory
}

func (b *BaseProvider) PortExtension() extensions.PortExtension {
	return b.portExtension
}

func (b *BaseProvider) NetworkExtension() extensions.NetworkExtension {
	return b.networkExtension
}
