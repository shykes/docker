package libnet

import (
	"net"
)

type DB struct {
	links []*Link
}

func Init(dir string) (*DB, error) {

}

func (db *DB) Add(

type Link struct {
	Consumer	NSID
	Name		string
	IP		*net.IP
	Config		Config
}

type Config interface {
	Get(string) (string, error)
	Set(string, string) error
	Delete(string) error
	List(string) ([]string, error)
}


// Potential link types:

// 1. Private bridge

// 2. Outbound Internet

// 3. Public port redirect (-p)

// 4. Macvlan

// 5. p2p veth

// 6. Custom bridge + veth + ip per container, determined by caller.
// (aka "the groupon use case")
