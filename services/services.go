

// A ServiceEngine is a collection of services
type ServiceEngine interface {
	ByName(name string) (Service, error)

	ServiceView
}

type ServiceView interface {
	LabelEquals(label, value string) ServiceView
	LabelLike(label, pattern string) ServiceView
	ByName(name string) (Service, error)
}


//

myservice.LabelEquals("service", "my")

// A service is a logical group of containers,
// which can be managed as a unit.
//
// Relationship between contianer and service:
// - A service includes 0-N containers
// - A container is part of exactly 1 service

//
// Organizing containers in a service:
// - Containers can be queried by label
// - Labels are key/value pairs chosen by the user
// - Each container may have any number of labels
type Service interface {//
	Containers() ([]*Container, error)
}


// FIXME: bfirsh: magical load-balancing? - 

// FIXME: bfirsh: labels

// FIXME: shykes: naming of services VS naming of containers VS links

// FIXME: shykes: are services like pods?
//		-> loose graph (current) vs strict graph


type ServiceStatus uint32
const (
	StatusUp	ServiceStatus = iota
	StatusDown
)

// Usage examples

// docker profit --help

//
