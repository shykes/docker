package simplebridge

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"strconv"
	"sync"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/docker/network"
	"github.com/docker/docker/pkg/iptables"
	"github.com/docker/docker/sandbox"
	"github.com/docker/docker/state"

	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netlink/nl"
)

const (
	maxVethName      = 10
	maxVethSuffixLen = 2
	maxVethSuffix    = 99
)

type BridgeDriver struct {
	schema *Schema
	state  state.State
	mutex  sync.Mutex
}

func (d *BridgeDriver) GetNetwork(network string) (network.Network, error) {
	return d.loadNetwork(network)
}

func (d *BridgeDriver) Restore(s state.State) error {
	d.state = s
	return nil
}

func (d *BridgeDriver) loadEndpoint(network, endpoint string) (*BridgeEndpoint, error) {
	scope := d.schema.Endpoint(network, endpoint)

	iface, err := scope.Get("interface_name")
	if err != nil {
		return nil, err
	}

	hwAddr, err := scope.Get("hwaddr")
	if err != nil {
		return nil, err
	}

	mtu, err := scope.Get("mtu")
	if err != nil {
		return nil, err
	}

	ipaddr, err := scope.Get("ip")
	if err != nil {
		return nil, err
	}

	ip := net.ParseIP(ipaddr)

	mtuInt, _ := strconv.ParseUint(mtu, 10, 32)

	netObj, err := d.loadNetwork(network)
	if err != nil {
		return nil, err
	}

	return &BridgeEndpoint{
		ID:            endpoint,
		interfaceName: iface,
		hwAddr:        hwAddr,
		mtu:           uint(mtuInt),
		network:       netObj,
		ip:            ip,
	}, nil
}

func (d *BridgeDriver) saveEndpoint(network string, ep *BridgeEndpoint) error {
	scope := d.schema.Endpoint(network, ep.ID)

	pathMap := map[string]string{
		"interface_name": ep.interfaceName,
		"hwaddr":         ep.hwAddr,
		"mtu":            strconv.Itoa(int(ep.mtu)),
		"ip":             ep.ip.String(),
	}

	return scope.MultiSet(pathMap)
}

// discovery driver? should it be hooked here or in the core?
func (d *BridgeDriver) Link(network, endpoint string, s sandbox.Sandbox, replace bool) (network.Endpoint, error) {
	if len(endpoint) > maxVethName {
		return nil, fmt.Errorf("endpoint %q is too long, must be %d characters", endpoint, maxVethName)
	}

	d.mutex.Lock()
	defer d.mutex.Unlock()

	netObj, err := d.loadNetwork(network)
	if err != nil {
		return nil, err
	}

	ep := &BridgeEndpoint{
		network: netObj,
		ID:      endpoint,
	}

	if ep, err := d.loadEndpoint(network, endpoint); ep != nil && err != nil && !replace {
		return nil, fmt.Errorf("Endpoint %q already taken", endpoint)
	}

	if err := d.schema.Endpoint(network, endpoint).Create(""); err != nil {
		fmt.Println("[fail] d.createEndpoint")
		return nil, err
	}

	if err := ep.configure(endpoint, s); err != nil {
		fmt.Printf("[fail] ep.configure: %v", err)
		return nil, err
	}

	if err := d.saveEndpoint(network, ep); err != nil {
		fmt.Println("[fail] d.saveEndpoint")
		return nil, err
	}

	return ep, nil
}

func (d *BridgeDriver) Unlink(netid, name string, sb sandbox.Sandbox) error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	ep, err := d.loadEndpoint(netid, name)
	if err != nil {
		return fmt.Errorf("No endpoint for name %q: %v", name, err)
	}

	if err := ep.deconfigure(name); err != nil {
		return err
	}

	if err := d.schema.Endpoint(netid, name).Remove(""); err != nil {
		return err
	}

	return nil
}

func (d *BridgeDriver) saveNetwork(network string, bridge *BridgeNetwork) error {
	networkSchema := d.schema.Network(network)
	// FIXME allocator, address will be broken if not saved
	if err := networkSchema.Set("bridgeInterface", bridge.bridge.Name); err != nil {
		return err
	}

	if err := networkSchema.Set("address", bridge.network.String()); err != nil {
		return err
	}

	return nil
}

func (d *BridgeDriver) loadNetwork(network string) (*BridgeNetwork, error) {
	networkSchema := d.schema.Network(network)

	iface, err := networkSchema.Get("bridgeInterface")
	if err != nil {
		return nil, err
	}

	addr, err := networkSchema.Get("address")
	if err != nil {
		return nil, err
	}

	ip, ipNet, err := net.ParseCIDR(addr)
	ipNet.IP = ip

	return &BridgeNetwork{
		// DEMO FIXME
		//vxlan:       &netlink.Vxlan{LinkAttrs: netlink.LinkAttrs{Name: "vx" + iface}},
		bridge:      &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: iface}},
		ID:          network,
		driver:      d,
		network:     ipNet,
		ipallocator: NewIPAllocator(iface, ipNet, nil, nil),
	}, nil
}

func (d *BridgeDriver) AddNetwork(network string, args []string) error {
	// FIXME this should be abstracted from the network driver

	fs := flag.NewFlagSet("simplebridge", flag.ContinueOnError)
	// FIXME need to figure out a way to prop usage
	fs.Usage = func() {}
	peer := fs.String("peer", os.Getenv("DOCKER_PEER"), "VXLan peer to contact")
	vlanid := fs.Uint("vid", 42, "VXLan VLAN ID")
	port := fs.Uint("port", 4789, "VXLan Tunneling Port")
	device := fs.String("dev", "eth0", "Device to set as the vxlan endpoint")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if err := d.schema.Network(network).Create(""); err != nil {
		return err
	}

	bridge, err := d.createBridge(network, *vlanid, *port, *peer, *device)
	if err != nil {
		return err
	}

	if err := d.saveNetwork(network, bridge); err != nil {
		return err
	}

	return nil
}

func (d *BridgeDriver) RemoveNetwork(network string) error {
	bridge, err := d.loadNetwork(network)
	if err != nil {
		return err
	}

	if err := d.schema.Network(network).Remove(""); err != nil {
		return err
	}

	return bridge.destroy()
}

func (d *BridgeDriver) getInterface(prefix string, linkParams netlink.Link) (netlink.Link, error) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	var (
		ethName   string
		available bool
	)

	for i := 0; i < maxVethSuffix; i++ {
		ethName = fmt.Sprintf("%s%d", prefix, i)
		if len(ethName) > maxVethName+maxVethSuffixLen {
			return nil, fmt.Errorf("EthName %q is longer than %d bytes", prefix, maxVethName)
		}
		if _, err := netlink.LinkByName(ethName); err != nil {
			available = true
			break
		}
	}

	if !available {
		return nil, fmt.Errorf("Cannot allocate more than %d ethernet devices for prefix %q", maxVethSuffix, prefix)
	}

	linkParams.Attrs().Name = ethName
	if err := netlink.LinkAdd(linkParams); err != nil {
		return nil, err
	}

	return linkParams, nil
}

func (d *BridgeDriver) createBridge(id string, vlanid uint, port uint, peer, device string) (*BridgeNetwork, error) {
	dockerbridge := &netlink.Bridge{netlink.LinkAttrs{Name: id}}

	linkval, err := d.getInterface(id, dockerbridge)
	if err != nil {
		log.Println("Error get interface", err)
		return nil, err
	}
	dockerbridge = linkval.(*netlink.Bridge)

	addr, err := GetBridgeIP()
	if err != nil {
		return nil, err
	}

	addrList, err := netlink.AddrList(dockerbridge, nl.GetIPFamily(addr.IP))
	if err != nil {
		return nil, err
	}

	var found bool
	for _, el := range addrList {
		if bytes.Equal(el.IPNet.IP, addr.IP) && bytes.Equal(el.IPNet.Mask, addr.Mask) {
			found = true
			break
		}
	}
	if !found {
		if err := netlink.AddrAdd(dockerbridge, &netlink.Addr{IPNet: addr}); err != nil {
			log.Println("Error add addr", err)
			return nil, err
		}
	}

	if err := netlink.LinkSetUp(dockerbridge); err != nil {
		log.Println("Error up bridge", err)
		return nil, err
	}

	if err := setupIPTables(id, addr, true, true); err != nil {
		return nil, err
	}

	var vxlan *netlink.Vxlan

	if peer != "" && device != "" && id != "default" { // FIXME DEMO default should not be treated this way
		iface, err := net.InterfaceByName(device)
		if err != nil {
			log.Println("Error get interface", err)
			return nil, err
		}

		vxlan = &netlink.Vxlan{
			// DEMO FIXME: name collisions, better error recovery
			LinkAttrs:    netlink.LinkAttrs{Name: "vx" + id, Flags: net.FlagMulticast},
			VtepDevIndex: iface.Index,
			VxlanId:      int(vlanid),
			Group:        net.ParseIP(peer),
			Port:         int(port),
		}

		linkval, err = d.getInterface(vxlan.LinkAttrs.Name, vxlan)
		if err != nil {
			log.Println("Error get interface", err)
			return nil, err
		}
		vxlan = linkval.(*netlink.Vxlan)

		// ignore errors in case it was already set
		if err := netlink.LinkSetMaster(vxlan, dockerbridge); err != nil {
			log.Println("Error linksetmaster", err)
			return nil, err
		}
		if err := netlink.LinkSetUp(vxlan); err != nil {
			log.Println("Error linksetmaster", err)
			return nil, err
		}
	}

	if err := MakeChain(id, dockerbridge.LinkAttrs.Name); err != nil {
		return nil, err
	}

	return &BridgeNetwork{
		vxlan:       vxlan,
		bridge:      dockerbridge,
		ID:          id,
		driver:      d,
		network:     addr,
		ipallocator: NewIPAllocator(dockerbridge.LinkAttrs.Name, addr, nil, nil),
	}, nil
}

func (d *BridgeDriver) destroyBridge(b *netlink.Bridge, v *netlink.Vxlan) error {
	// DEMO FIXME
	if v != nil {
		if err := netlink.LinkDel(v); err != nil {
			return err
		}
	}

	return netlink.LinkDel(b)
}

// FIXME remove last two parameters
func setupIPTables(bridgeIface string, addr net.Addr, icc, ipmasq bool) error {

	if err := ioutil.WriteFile("/proc/sys/net/ipv4/ip_forward", []byte("1"), 0600); err != nil {
		return err
	}

	// Enable NAT

	if ipmasq {
		natArgs := []string{"POSTROUTING", "-t", "nat", "-s", addr.String(), "!", "-o", bridgeIface, "-j", "MASQUERADE"}

		if !iptables.Exists(natArgs...) {
			if output, err := iptables.Raw(append([]string{"-I"}, natArgs...)...); err != nil {
				return fmt.Errorf("Unable to enable network bridge NAT: %s", err)
			} else if len(output) != 0 {
				return &iptables.ChainError{Chain: "POSTROUTING", Output: output}
			}
		}
	}

	var (
		args       = []string{"FORWARD", "-i", bridgeIface, "-o", bridgeIface, "-j"}
		acceptArgs = append(args, "ACCEPT")
		dropArgs   = append(args, "DROP")
	)

	if !icc {
		iptables.Raw(append([]string{"-D"}, acceptArgs...)...)

		if !iptables.Exists(dropArgs...) {
			log.Debugf("Disable inter-container communication")
			if output, err := iptables.Raw(append([]string{"-I"}, dropArgs...)...); err != nil {
				return fmt.Errorf("Unable to prevent intercontainer communication: %s", err)
			} else if len(output) != 0 {
				return fmt.Errorf("Error disabling intercontainer communication: %s", output)
			}
		}
	} else {
		iptables.Raw(append([]string{"-D"}, dropArgs...)...)

		if !iptables.Exists(acceptArgs...) {
			log.Debugf("Enable inter-container communication")
			if output, err := iptables.Raw(append([]string{"-I"}, acceptArgs...)...); err != nil {
				return fmt.Errorf("Unable to allow intercontainer communication: %s", err)
			} else if len(output) != 0 {
				return fmt.Errorf("Error enabling intercontainer communication: %s", output)
			}
		}
	}

	// Accept all non-intercontainer outgoing packets
	outgoingArgs := []string{"FORWARD", "-i", bridgeIface, "!", "-o", bridgeIface, "-j", "ACCEPT"}
	if !iptables.Exists(outgoingArgs...) {
		if output, err := iptables.Raw(append([]string{"-I"}, outgoingArgs...)...); err != nil {
			return fmt.Errorf("Unable to allow outgoing packets: %s", err)
		} else if len(output) != 0 {
			return &iptables.ChainError{Chain: "FORWARD outgoing", Output: output}
		}
	}

	// Accept incoming packets for existing connections
	existingArgs := []string{"FORWARD", "-o", bridgeIface, "-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT"}

	if !iptables.Exists(existingArgs...) {
		if output, err := iptables.Raw(append([]string{"-I"}, existingArgs...)...); err != nil {
			return fmt.Errorf("Unable to allow incoming packets: %s", err)
		} else if len(output) != 0 {
			return &iptables.ChainError{Chain: "FORWARD incoming", Output: output}
		}
	}
	return nil
}
