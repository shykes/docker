package simplebridge

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net"

	"github.com/docker/docker/pkg/iptables"

	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netlink/nl"
)

func bridgeError(typ string, err error) error {
	return fmt.Errorf("createBridge: %s: %v", typ, err)
}

func (d *BridgeDriver) createBridge(id string, vlanid uint, port uint, peer, device string, force bool) (*BridgeNetwork, error) {
	dockerbridge := &netlink.Bridge{netlink.LinkAttrs{Name: id}}

	linkval, err := d.getInterface(id, dockerbridge, force)
	if err != nil {
		return nil, err
	}
	dockerbridge = linkval.(*netlink.Bridge)

	addr, err := GetBridgeIP()
	if err != nil {
		return nil, err
	}

	addrList, err := netlink.AddrList(dockerbridge, nl.GetIPFamily(addr.IP))
	if err != nil {
		return nil, bridgeError("list addresses for bridge", err)
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
			return nil, bridgeError("bridge add address", err)
		}
	}

	if err := netlink.LinkSetUp(dockerbridge); err != nil {
		return nil, bridgeError("bridge interface up", err)
	}

	if err := setupIPTables(id, addr); err != nil {
		return nil, bridgeError("configure iptables", err)
	}

	var vxlan *netlink.Vxlan

	if peer != "" && device != "" {
		iface, err := net.InterfaceByName(device)
		if err != nil {
			return nil, bridgeError("retrieve interface name", err)
		}

		vxlan = &netlink.Vxlan{
			// DEMO FIXME: name collisions, better error recovery
			LinkAttrs:    netlink.LinkAttrs{Name: "vx" + id, Flags: net.FlagMulticast},
			VtepDevIndex: iface.Index,
			VxlanId:      int(vlanid),
			Group:        net.ParseIP(peer),
			Port:         int(port),
		}

		linkval, err = d.getInterface(vxlan.LinkAttrs.Name, vxlan, force)
		if err != nil {
			return nil, bridgeError("retrieve interface name", err)
		}
		vxlan = linkval.(*netlink.Vxlan)

		// ignore errors in case it was already set
		if err := netlink.LinkSetMaster(vxlan, dockerbridge); err != nil {
			return nil, bridgeError("add vxlan interface to bridge", err)
		}
		if err := netlink.LinkSetUp(vxlan); err != nil {
			return nil, bridgeError("vxlan interface up", err)
		}
	}

	if err := MakeChain(id, dockerbridge.LinkAttrs.Name); err != nil {
		return nil, bridgeError("bootstrap iptables chain for bridge", err)
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
			return fmt.Errorf("vxlan link delete: %v", err)
		}
	}

	if err := netlink.LinkDel(b); err != nil {
		return fmt.Errorf("bridge link del: %v", err)
	}

	return nil
}

func (d *BridgeDriver) createInterface(linkParams netlink.Link) (netlink.Link, error) {
	if err := netlink.LinkAdd(linkParams); err != nil {
		return nil, err
	}
	return linkParams, nil
}

func (d *BridgeDriver) assertInterface(interfaceName string) bool {
	_, err := netlink.LinkByName(interfaceName)
	return err == nil
}

func (d *BridgeDriver) getInterface(prefix string, linkParams netlink.Link, create bool) (netlink.Link, error) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	var (
		ethName   string
		available bool
	)

	if create {
		if len(prefix) > maxVethName {
			return nil, fmt.Errorf("getInterface: prefix %q is longer than %d bytes", prefix, maxVethName)
		}

		for i := 0; i < maxVethSuffix; i++ {
			ethName = fmt.Sprintf("%s%d", prefix, i)
			// this needs to be tested here because the suffix length is variable
			if len(ethName) > maxVethName+maxVethSuffixLen {
				return nil, fmt.Errorf("getInterface: EthName %q is longer than %d bytes", prefix, maxVethName+maxVethSuffixLen)
			}
			// FIXME create the interface here so it's atomic
			if !d.assertInterface(ethName) {
				available = true
				break
			}
		}

		if !available {
			return nil, fmt.Errorf("getInterface: Cannot allocate more than %d ethernet devices for prefix %q", maxVethSuffix, prefix)
		}

		linkParams.Attrs().Name = ethName
		return d.createInterface(linkParams)
	} else if !d.assertInterface(linkParams.Attrs().Name) {
		// if create is not specified, and the interface doesn't exist, try to
		// create it with the current link paramters.
		return d.createInterface(linkParams)
	}

	// if create is not specified, and the interface exists, this will return
	// exactly what was given to us.
	return linkParams, nil
}

func setupIPTables(bridgeIface string, addr net.Addr) error {
	if err := ioutil.WriteFile("/proc/sys/net/ipv4/ip_forward", []byte("1"), 0600); err != nil {
		return fmt.Errorf("Setting net.ipv4.ip_forward: %v", err)
	}

	// Enable NAT

	natArgs := []string{"POSTROUTING", "-t", "nat", "-s", addr.String(), "!", "-o", bridgeIface, "-j", "MASQUERADE"}

	if !iptables.Exists(natArgs...) {
		if output, err := iptables.Raw(append([]string{"-I"}, natArgs...)...); err != nil {
			return fmt.Errorf("Unable to enable network bridge NAT: %s", err)
		} else if len(output) != 0 {
			return &iptables.ChainError{Chain: "POSTROUTING", Output: output}
		}
	}

	var (
		args       = []string{"FORWARD", "-i", bridgeIface, "-o", bridgeIface, "-j"}
		acceptArgs = append(args, "ACCEPT")
		dropArgs   = append(args, "DROP")
	)

	iptables.Raw(append([]string{"-D"}, dropArgs...)...)

	if !iptables.Exists(acceptArgs...) {
		if output, err := iptables.Raw(append([]string{"-I"}, acceptArgs...)...); err != nil {
			return fmt.Errorf("Unable to allow intercontainer communication: %s", err)
		} else if len(output) != 0 {
			return fmt.Errorf("Error enabling intercontainer communication: %s", output)
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
