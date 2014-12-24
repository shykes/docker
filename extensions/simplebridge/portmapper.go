package simplebridge

import (
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path"
	"regexp"
	"strings"
	"sync"

	"github.com/docker/docker/pkg/iptables"
)

type PortMap struct {
	chainName     string
	proto         string
	hostIP        net.IP
	hostPort      uint
	containerIP   net.IP
	containerPort uint
	forward       forwardFunc
}

// since managing ports is a global operation on the host, we need to ensure we
// use a global lock when reading the /proc/net files.
//
// XXX in all honestly, I'm not sure this belongs here, and instead belongs in
// docker proper. Not sure yet.

var mapperMutex sync.Mutex
var chainMutex sync.Mutex // iptables are the same way

var (
	chainMap        = map[string]*iptables.Chain{}
	hostPortMap     = map[uint]struct{}{} // locked with mapperMutex in Map and Unmap
	portTableFormat = "/proc/net/%s"      // XXX this is overwritten in tests to use test data.
)

type forwardFunc func(string, iptables.Action, string, net.IP, uint, net.IP, uint) error

func getIPTablesChain(chainName string) *iptables.Chain {
	// XXX locks are handled in forward()
	return chainMap[chainName]
}

func forward(chainName string, action iptables.Action, proto string, sourceIP net.IP, sourcePort uint, containerIP net.IP, containerPort uint) error {
	chainMutex.Lock()
	defer chainMutex.Unlock()
	chain := getIPTablesChain(chainName)
	return chain.Forward(action, sourceIP, sourcePort, proto, containerIP, containerPort)
}

func loadPortTable(proto string, mapped map[uint][]net.IP) error {
	f, err := os.Open(fmt.Sprintf(portTableFormat, path.Base(proto)))
	if err != nil {
		return fmt.Errorf("Error scanning local port mappings: %v", err)
	}

	defer f.Close()

	content, err := ioutil.ReadAll(f)
	if err != nil {
		return fmt.Errorf("Error scanning local port mappings: %v", err)
	}

	strContent := string(content)

	lines := strings.Split(strContent, "\n")
	for _, line := range lines[1:] {
		parts := regexp.MustCompile(`\s+`).Split(line, -1)
		if len(parts) < 3 {
			continue
		}

		parts = strings.SplitN(parts[2], ":", 2)

		ip, err := hex.DecodeString(parts[0])
		if err != nil {
			return fmt.Errorf("Error scanning local port mappings: %v", err)
		}

		port, err := hex.DecodeString(parts[1])
		if err != nil {
			return fmt.Errorf("Error scanning local port mappings: %v", err)
		}

		realIP := net.IP(ip)

		uintPort := uint(0)

		for i := 0; i < len(port); i++ {
			uintPort = uintPort<<8 | uint(port[i])&0xFF
		}

		if _, ok := mapped[uintPort]; ok {
			mapped[uintPort] = append(mapped[uintPort], realIP)
		} else {
			mapped[uintPort] = []net.IP{realIP}
		}
	}

	return nil
}

func MakeChain(chainName string, networkName string) error {
	chainMutex.Lock()
	defer chainMutex.Unlock()

	// Recreate the chain if it already exists
	chain, err := iptables.NewChain(chainName, networkName)
	if err != nil {
		chainMap[chainName] = &iptables.Chain{chainName, networkName}
		chainMap[chainName].Remove()
		chainMap[chainName], err = iptables.NewChain(chainName, networkName)
		if err != nil {
			return fmt.Errorf("MakeChain: %v", err)
		}
	} else {
		chainMap[chainName] = chain
	}

	return nil
}

func NewPortMap(chainName string, hostIP net.IP, proto string, containerIP net.IP, containerPort, hostPort uint, fwd forwardFunc) *PortMap {
	if fwd == nil {
		fwd = forward
	}

	return &PortMap{
		chainName:     chainName,
		proto:         proto,
		hostIP:        hostIP,
		hostPort:      hostPort,
		containerIP:   containerIP,
		containerPort: containerPort,
		forward:       fwd,
	}
}

func (pm *PortMap) Unmap() error {
	mapperMutex.Lock()
	defer mapperMutex.Unlock()

	if err := pm.forward(pm.chainName, iptables.Delete, pm.proto, pm.hostIP, pm.hostPort, pm.containerIP, pm.containerPort); err != nil {
		return fmt.Errorf("Error unmapping %s port from %s:%s -> %s:%s: %v", pm.proto, pm.hostIP, pm.hostPort, pm.containerIP, pm.containerPort, err)
	}

	delete(hostPortMap, pm.hostPort)

	return nil
}

func (pm *PortMap) Map() error {
	mapperMutex.Lock()
	defer mapperMutex.Unlock()

	mapped := map[uint][]net.IP{}

	if err := loadPortTable(pm.proto, mapped); err != nil {
		return err
	}

	if err := loadPortTable(pm.proto+"6", mapped); err != nil {
		return err
	}

	ips, ok := mapped[pm.hostPort]

	if ok {
		if pm.hostIP.String() == "0.0.0.0" || pm.hostIP.String() == "::" {
			return fmt.Errorf("Port %d cannot be mapped because %q cannot be used exclusively", pm.hostIP.String())
		}

		switch {
		case pm.hostIP.To4() != nil:
			for _, ip := range ips {
				if ip.To4() != nil {
					if pm.hostIP.Equal(ip) || ip.String() == "0.0.0.0" {
						return fmt.Errorf("Port %d cannot be mapped because it is already in use by %q", pm.hostPort, pm.hostIP.String())
					}
				}
			}
		case pm.hostIP.To16() != nil:
			for _, ip := range ips {
				if ip.To16() != nil {
					if pm.hostIP.Equal(ip) || ip.String() == "::" {
						return fmt.Errorf("Port %d cannot be mapped because it is already in use by %q", pm.hostPort, pm.hostIP.String())
					}
				}
			}
		default:
			return fmt.Errorf("IP %q is not a valid IP address", pm.hostIP.String())
		}
	} else {
		if err := pm.forward(pm.chainName, iptables.Add, pm.proto, pm.hostIP, pm.hostPort, pm.containerIP, pm.containerPort); err != nil {
			return fmt.Errorf("Error mapping %s port from %s:%s -> %s:%s: %v", pm.proto, pm.hostIP, pm.hostPort, pm.containerIP, pm.containerPort, err)
		}
	}

	// since the forward rules won't show up in the bound ports files we need to
	// track this independently.
	hostPortMap[pm.hostPort] = struct{}{}

	return nil
}
