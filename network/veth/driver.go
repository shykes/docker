package veth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"strconv"
	"sync"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/docker/daemon/networkdriver/ipallocator"
	"github.com/docker/docker/daemon/networkdriver/portmapper"
	"github.com/docker/docker/network"
	"github.com/docker/docker/pkg/iptables"
	"github.com/docker/docker/pkg/netlink"
	"github.com/docker/docker/pkg/networkfs/resolvconf"
)

const (
	DefaultBridge = "docker0"
	DefaultMtu    = 1500
	VethPrefix    = "veth"
)

type Config struct {
	Iface          string
	CIDR           string
	FixedCIDR      string
	EnableIPTables bool
	ICC            bool
	IPMasq         bool
	IPForward      bool
}

type Bridge struct {
	Iface     string
	Net       *net.IPNet
	FixedNet  *net.IPNet
	endpoints map[string]string
	mu        sync.Mutex
}

func getNetwork(iface string, cidr string) (*net.IPNet, error) {
	if cidr != "" {
		ip, network, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, err
		}
		network.IP = ip
		return network, nil
	}
	var nameservers []string
	resolvConf, err := resolvconf.Get()
	if err == nil {
		nameservers = resolvconf.GetNameserversAsCIDR(resolvConf)
	}
	for _, n := range networks {
		if err := network.CheckNameserverOverlaps(nameservers, n); err != nil {
			continue
		}
		if err := network.CheckRouteOverlaps(n); err != nil {
			continue
		}
		return n, nil
	}
	return nil, fmt.Errorf("Could not find a free IP address range for interface '%s'. Please configure its address manually and run 'docker -b %s'", iface, iface)
}

func New(config Config) (*Bridge, error) {
	var bridgeNet *net.IPNet
	if config.Iface == "" {
		config.Iface = DefaultBridge
	}
	b := &Bridge{
		Iface:     config.Iface,
		endpoints: make(map[string]string),
	}
	addr, err := network.GetIfaceAddr(config.Iface)
	if err != nil {
		// If we're not using the default bridge, fail without trying to create it
		if config.Iface != DefaultBridge {
			return nil, err
		}
		bridgeNet, err = getNetwork(config.Iface, config.CIDR)
		if err != nil {
			return nil, err
		}
		log.Debugf("Creating bridge %s with network %s", config.Iface, bridgeNet)
		if err := configureBridge(config.Iface, bridgeNet); err != nil {
			return nil, err
		}
		addr, err = network.GetIfaceAddr(config.Iface)
		if err != nil {
			return nil, err
		}
	} else {
		bridgeNet = addr.(*net.IPNet)
		if config.CIDR != "" {
			bip, _, err := net.ParseCIDR(config.CIDR)
			if err != nil {
				return nil, err
			}
			if !bridgeNet.IP.Equal(bip) {
				return nil, fmt.Errorf("bridge ip (%s) does not match existing bridge configuration %s", bridgeNet.IP, bip)
			}
		}
	}
	b.Net = bridgeNet

	if config.IPForward {
		// Enable IPv4 forwarding
		if err := ioutil.WriteFile("/proc/sys/net/ipv4/ip_forward", []byte{'1', '\n'}, 0644); err != nil {
			log.Infof("WARNING: unable to enable IPv4 forwarding: %s\n", err)
		}
	}
	// We can always try removing the iptables
	if err := iptables.RemoveExistingChain("DOCKER"); err != nil {
		return nil, err
	}
	// Configure iptables for link support
	if config.EnableIPTables {
		if err := setupIPTables(config.Iface, addr, config.ICC, config.IPMasq); err != nil {
			return nil, err
		}
		chain, err := iptables.NewChain("DOCKER", config.Iface)
		if err != nil {
			return nil, err
		}
		portmapper.SetIptablesChain(chain)
	}

	if config.FixedCIDR != "" {
		_, subnet, err := net.ParseCIDR(config.FixedCIDR)
		if err != nil {
			return nil, err
		}
		log.Debugf("Subnet: %v", subnet)
		if err := ipallocator.RegisterSubnet(b.Net, subnet); err != nil {
			return nil, err
		}
		b.FixedNet = subnet
	}

	return b, nil
}

func (b *Bridge) AddEndpoint(netId, cId string, config map[string]string) ([]network.Interface, error) {
	var (
		mac         net.HardwareAddr
		ip          net.IP
		mtu         int
		err         error
		requestedIP = net.ParseIP(config["IP"])
		txQueueLen  int
	)

	// If no explicit mac address was given, generate a random one.
	if mac, err = net.ParseMAC(config["Mac"]); err != nil {
		mac = network.GenerateMacAddr(ip)
	}

	if mtu, err = strconv.Atoi(config["Mtu"]); err != nil {
		mtu = DefaultMtu
	}

	if txQueueLen, err = strconv.Atoi(config["TxQueueLen"]); err != nil {
		txQueueLen = 0
	}

	if requestedIP != nil {
		ip, err = ipallocator.RequestIP(b.Net, requestedIP)
	} else {
		ip, err = ipallocator.RequestIP(b.Net, nil)
	}
	if err != nil {
		return nil, err
	}

	cidr := (&net.IPNet{
		IP:   ip,
		Mask: b.Net.Mask,
	}).String()

	parent, child, err := b.getVethPair(mtu, txQueueLen)
	if err != nil {
		return nil, err
	}
	iface := network.Interface{
		Iface:      child,
		Address:    cidr,
		Gateway:    b.Net.IP.String(),
		MacAddress: mac.String(),
		Mtu:        mtu,
	}
	b.mu.Lock()
	b.endpoints[cId] = parent
	b.mu.Unlock()

	return []network.Interface{iface}, nil

	//size, _ := bridgeNetwork.Mask.Size()
	//out.SetInt("IPPrefixLen", size)
}

func (b *Bridge) RemoveEndpoint(id string) error {
	b.mu.Lock()
	link, ok := b.endpoints[id]
	if !ok {
		return nil
	}
	delete(b.endpoints, id)
	b.mu.Unlock()
	return netlink.NetworkLinkDel(link)
}

func generateRandomName(prefix string, size int) (string, error) {
	id := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, id); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(id)[:size], nil
}

func createVethPair(prefix string, txQueueLen int) (name1 string, name2 string, err error) {
	for i := 0; i < 10; i++ {
		if name1, err = generateRandomName(prefix, 7); err != nil {
			return
		}

		if name2, err = generateRandomName(prefix, 7); err != nil {
			return
		}

		if err = network.CreateVethPair(name1, name2, txQueueLen); err != nil {
			if err == netlink.ErrInterfaceExists {
				continue
			}
			return
		}
		break
	}
	return
}

func (b *Bridge) getVethPair(mtu, txQueueLen int) (string, string, error) {
	name1, name2, err := createVethPair(VethPrefix, txQueueLen)
	if err != nil {
		return "", "", err
	}
	if err := network.SetInterfaceMaster(name1, b.Iface); err != nil {
		return "", "", err
	}
	if err := network.SetMtu(name1, mtu); err != nil {
		return "", "", err
	}
	if err := network.InterfaceUp(name1); err != nil {
		return "", "", err
	}
	return name1, name2, nil
}
