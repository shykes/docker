package simplebridge

import (
	"fmt"
	"io/ioutil"
	"net"

	"github.com/docker/docker/pkg/iptables"

	"github.com/vishvananda/netlink"
)

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

func setupIPTables(bridgeIface string, addr net.Addr) error {
	if err := ioutil.WriteFile("/proc/sys/net/ipv4/ip_forward", []byte("1"), 0600); err != nil {
		return err
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
