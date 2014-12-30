package simplebridge

import (
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/erikh/ping"
)

var bridgeAddrs = []string{
	// Here we don't follow the convention of using the 1st IP of the range for the gateway.
	// This is to use the same gateway IPs as the /24 ranges, which predate the /16 ranges.
	// In theory this shouldn't matter - in practice there's bound to be a few scripts relying
	// on the internal addressing or other stupid things like that.
	// They shouldn't, but hey, let's not break them unless we really have to.
	//
	// Don't use 172.16.0.0/16, it conflicts with EC2 DNS 172.16.0.23
	//
	"10.42.42.1/16",
	"10.1.42.1/16",

	// XXX this next line was changed from a /16 to /24 because the netmask would
	// allow for EC2's DNS to be trumped still. The 10.x/16's were moved to the top
	// as a result of this.
	"172.17.42.1/24",
	"172.16.42.1/24",
	"172.16.43.1/24",
	"172.16.44.1/24",
	"10.0.42.1/24",
	"10.0.43.1/24",
	"192.168.42.1/24",
	"192.168.43.1/24",
	"192.168.44.1/24",
}

// FIXME have this accept state objects to get at parameter data
func GetBridgeIP() (*net.IPNet, error) {
	for _, addr := range bridgeAddrs {
		ip, ipNet, err := net.ParseCIDR(addr)
		if err != nil {
			return nil, fmt.Errorf("bridge allocator error: %v", err)
		}

		if !ping.Ping(&net.IPAddr{ip, ""}, 150*time.Millisecond) {
			ipNet.IP = ip // set the bridge IP to the one we want
			return ipNet, nil
		}
	}

	return nil, errors.New("Could not find a suitable bridge IP!")
}
