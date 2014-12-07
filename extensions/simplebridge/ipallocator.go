package simplebridge

import (
	"fmt"
	"math/big"
	"net"
	"sync"

	log "github.com/Sirupsen/logrus"
	"github.com/vishvananda/netlink"
)

// XXX I'm also wondering if, with the approrpriate hooks, the ipallocator
// could also be used outside simplebridge.

type refreshFunc func(*net.Interface) (map[string]struct{}, error)

type IPAllocator struct {
	bridgeName  string
	bridgeNet   *net.IPNet
	lastIP      net.IP
	v6          bool
	refreshFunc refreshFunc
	mutex       sync.Mutex
}

func NewIPAllocator(bridgeName string, bridgeNet *net.IPNet, refreshFunc refreshFunc) *IPAllocator {
	ip := &IPAllocator{
		bridgeName:  bridgeName,
		bridgeNet:   bridgeNet,
		lastIP:      bridgeNet.IP,
		v6:          bridgeNet.IP.To4() == nil,
		refreshFunc: refreshFunc,
	}

	if refreshFunc == nil {
		ip.refreshFunc = ip.refresh
	}

	return ip
}

func (ip *IPAllocator) refresh(_if *net.Interface) (map[string]struct{}, error) {
	var (
		list []netlink.Neigh
		err  error
	)

	if ip.v6 {
		list, err = netlink.NeighList(_if.Index, netlink.FAMILY_V6)
		if err != nil {
			return nil, err
		}
	} else {
		list, err = netlink.NeighList(_if.Index, netlink.FAMILY_V4)
		if err != nil {
			return nil, err
		}
	}

	ipMap := map[string]struct{}{}

	for _, entry := range list {
		ipMap[entry.String()] = struct{}{}
	}

	return ipMap, nil
}

func (ip *IPAllocator) Refresh() (map[string]struct{}, error) {
	_if, err := net.InterfaceByName(ip.bridgeName)
	if err != nil {
		return nil, err
	}

	return ip.refreshFunc(_if)
}

func (ip *IPAllocator) Allocate() (net.IP, error) {
	// FIXME use netlink package to insert into the neighbors table / arp cache
	ip.mutex.Lock()
	defer ip.mutex.Unlock()

	var (
		newip  net.IP
		ok     bool
		cycled bool
	)

	ipMap, err := ip.Refresh()
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
			ipMap[newip.String()] = struct{}{}
			ip.lastIP = newip
			break
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
