package simplebridge

import (
	"fmt"
	"math/big"
	"net"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/erikh/ping"
	"github.com/vishvananda/netlink"
)

// XXX I'm also wondering if, with the approrpriate hooks, the ipallocator
// could also be used outside simplebridge.

type refreshFunc func(*net.Interface) (map[string]struct{}, error)
type allocateFunc func(net.IP) bool

type IPAllocator struct {
	bridgeName   string
	bridgeNet    *net.IPNet
	lastIP       net.IP
	v6           bool
	refreshFunc  refreshFunc
	allocateFunc allocateFunc
	mutex        sync.Mutex
}

func NewIPAllocator(bridgeName string, bridgeNet *net.IPNet, refreshFunc refreshFunc, allocateFunc allocateFunc) *IPAllocator {
	ip := &IPAllocator{
		bridgeName:   bridgeName,
		bridgeNet:    bridgeNet,
		lastIP:       bridgeNet.IP,
		v6:           bridgeNet.IP.To4() == nil,
		refreshFunc:  refreshFunc,
		allocateFunc: allocateFunc,
	}

	if refreshFunc == nil {
		ip.refreshFunc = ip.refresh
	}

	if allocateFunc == nil {
		ip.allocateFunc = ip.allocate
	}

	return ip
}

func (ip *IPAllocator) allocate(dstIP net.IP) bool {
	return !ping.Ping(&net.IPAddr{dstIP, ""}, 150*time.Millisecond)
}

func (ip *IPAllocator) refresh(_if *net.Interface) (map[string]struct{}, error) {
	var (
		list []netlink.Neigh
		err  error
	)

	if ip.v6 {
		list, err = netlink.NeighList(_if.Index, netlink.FAMILY_V6)
		if err != nil {
			return nil, fmt.Errorf("Cannot retrieve IPv6 neighbor information for interface %q: %v", _if.Name, err)
		}
	} else {
		list, err = netlink.NeighList(_if.Index, netlink.FAMILY_V4)
		if err != nil {
			return nil, fmt.Errorf("Cannot retrieve IPv4 neighbor information for interface %q: %v", _if.Name, err)
		}
	}

	ipMap := map[string]struct{}{}

	for _, entry := range list {
		ipMap[entry.String()] = struct{}{}
	}

	return ipMap, nil
}

func (ip *IPAllocator) Allocate() (net.IP, error) {
	ip.mutex.Lock()
	defer ip.mutex.Unlock()

	var (
		newip  net.IP
		ok     bool
		cycled bool
	)

	_if, err := net.InterfaceByName(ip.bridgeName)
	if err != nil {
		return nil, fmt.Errorf("ipallocator fetch bridge %q: %v", ip.bridgeName, err)
	}

	ipMap, err := ip.refreshFunc(_if)
	if err != nil {
		return nil, err
	}

	lastip := ip.bridgeNet.IP

	for {
		rawip := ipToBigInt(lastip)

		rawip.Add(rawip, big.NewInt(1))
		newip = bigIntToIP(rawip)

		if !ip.bridgeNet.Contains(newip) {
			if cycled {
				return nil, fmt.Errorf("Could not find a suitable IP for network %q", ip.bridgeNet.String())
			}

			lastip = ip.bridgeNet.IP
			cycled = true
		}

		_, ok = ipMap[newip.String()]
		if !ok {
			// use ICMP to check if the IP is in use, final sanity check.
			if ip.allocateFunc(newip) {
				ipMap[newip.String()] = struct{}{}
				ip.lastIP = newip
				break
			} else if err != nil {
				return nil, err
			}
		}

		lastip = newip
	}

	return newip, nil
}

// Converts a 4 bytes IP into a 128 bit integer
func ipToBigInt(ip net.IP) *big.Int {
	x := big.NewInt(0)
	if ip4 := ip.To4(); ip4 != nil {
		return x.SetBytes(ip4)
	}
	if ip6 := ip.To16(); ip6 != nil {
		return x.SetBytes(ip6)
	}

	log.Errorf("ipToBigInt: Wrong IP length! %s", ip)
	return nil
}

// Converts 128 bit integer into a 4 bytes IP address
func bigIntToIP(v *big.Int) net.IP {
	return net.IP(v.Bytes())
}
