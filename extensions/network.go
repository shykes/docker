package extensions

import (
	"net"
)

const (
	CONTAINER_ID = "ContainerId"
)

// NetworkProvider is a composition of various extensions to provide networking
// functionality.  When a network is created, a provider can be chosen at that
// type.
type NetworkProvider interface {
	// Opts returns a specification of options recognized by the extension.
	// The runtime uses this schema to query users for opts when loading extensionss.
	// Opts are made available to the extensions under the key "/opts" in its State.
	//
	// Schema example:
	// []Opt{
	//	{"key", "string", "A secret key to encrypt all traffic"},
	//	{"bridge", "string", "The name of the bridge interface to configure"},
	//	{"autoaddr", "bool", "Auto-detect a network range if the bridge is not already configured"},
	// }
	//Opts() []Opt

	Name() string
	NetworkExtension() NetworkExtension
	NamingExtension() NamingExtension
	PortExtension() PortExtension
	GetNetwork(idOrName string) (Network, error)

	Init(s State) error
	Shutdown(s State) error
}

type NamingExtension interface {
	AssignNamesInNetwork(network Network, names map[string]*Endpoint) error

	// This is to support links in which the link names are scoped to the container
	AssignNamesInContainer(containerId string, names map[string]*Endpoint) error
}

type PortExtension interface {
	ExposePort(endpoint *Endpoint, network Network, port *Port, hostPort int) error
	UnexposePort(endpoint *Endpoint, network Network, port *Port, hostPort int) error
}

// I assume this type exists somewhere else too
type Port struct {
	Port     int
	Protocol string
}

// NetExtension is the interface implemented by a network extension.
// Network extensions implement new ways for Docker to interconnect containers
// over IP networks.

type NetworkExtension interface {
	ListNetworks() ([]Network, error)

	GetNetwork(id string) (Network, error)

	// Creates a network
	// The implementation of this method should be idempotent such that if
	// then network already exists it will succeed
	CreateNet(net Network) error

	// Removes a network, this should be idempotent such that if the network
	// does not if exist it should succeed
	RemoveNet(net Network) error
}

type NamespaceFactory interface {
	IsHost() bool
	IsContainer() bool
	GetNamespace() string //or whatever type a namespace is
}

type Network interface {
	Id() string
	NamespaceFactory() NamespaceFactory
	CustomDnsSupported() bool
	Provider() NetworkProvider
	ListEndpoints() ([]*Endpoint, error)
	LookupEndpoint(metaData map[string]string) (*Endpoint, error)
	// Called on container create
	CreateEndpoint(endpoint *Endpoint) error
	// Called on container start
	ActivateEndpoint(endpoint *Endpoint) error
	// Called on container stop
	DeactivateEndpoint(endpoint *Endpoint) error
	// Called on container remove
	RemoveEndpoint(endpoint *Endpoint) error
}

type Endpoint struct {
	Id         string
	Interfaces []*Interface
	MetaData   map[string]string
}

type Opt struct {
	Key  string
	Type string
	Desc string
}

// this is a network configuration provided by the extension at network creation
// time. it is not intended to be malleable by docker itself, but should be
// populated with all the generic configuration required by dockerinit to work
// with the container from a network perspective.
type Interface struct {
	net.Interface
	Addresses []string // a list of CIDR addresses to assign to the interface
	Gateway   net.IP   // the gateway the interface uses
	Routes    []Route  // routes to set on this interface
}

type Route struct {
	Target  string // The CIDR address of the route target
	Gateway string // The CIDR address of the gateway (empty string means no gateway)
}

// State is an abstract database provided by the core for a extension to easily store
// and retrieve its state across its lifecycle.
//
// State data is organized like a simplified filesystem: nodes in the tree
// are referenced by slash-separated paths. A node can be either a directory
// or a file. Files have no metadata, only data.
//
// An important property of State is full support of versioning and transactions.
// All changes return a globally unique hash of the current state of the database.
//
// State supports transactions:
// [...]
// var (
//    s State
//    err error
//  )
// s.Autocommit(false)
// s, _ = s.("/foo/bar")
// s, _ = s.Set("/animals/moby dock", "Moby Dock is a whale")
// _ = s.Commit()

type State interface {
	List(dir string) ([]string, error)
	Get(key string) (string, error)

	// FIXME: for now we stick to naive crud,
	// bring back transactions and versioning once
	// we have a working poc.
	Set(key, val string) error
	Remove(key string) error
	Mkdir(dir string) error
}
