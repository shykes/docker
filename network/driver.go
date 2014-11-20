package network

type Driver interface {
	AddEndpoint(string, string, map[string]string) ([]Interface, error)
	RemoveEndpoint(string) error
}
