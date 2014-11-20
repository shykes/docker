package veth

import (
	"fmt"
	"net"
	"os"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/docker/pkg/iptables"
	"github.com/docker/docker/pkg/netlink"
	"github.com/docker/docker/pkg/parsers/kernel"
)

var networks []*net.IPNet

func init() {
	// Here we don't follow the convention of using the 1st IP of the range for the gateway.
	// This is to use the same gateway IPs as the /24 ranges, which predate the /16 ranges.
	// In theory this shouldn't matter - in practice there's bound to be a few scripts relying
	// on the internal addressing or other stupid things like that.
	// They shouldn't, but hey, let's not break them unless we really have to.
	for _, addr := range []string{"172.17.42.1/16", // Don't use 172.16.0.0/16, it conflicts with EC2 DNS 172.16.0.23
		"10.0.42.1/16", // Don't even try using the entire /8, that's too intrusive
		"10.1.42.1/16",
		"10.42.42.1/16",
		"172.16.42.1/24",
		"172.16.43.1/24",
		"172.16.44.1/24",
		"10.0.42.1/24",
		"10.0.43.1/24",
		"192.168.42.1/24",
		"192.168.43.1/24",
		"192.168.44.1/24",
	} {
		ip, net, err := net.ParseCIDR(addr)
		if err != nil {
			log.Errorf("Failed to parse address %s", addr)
			continue
		}
		net.IP = ip
		networks = append(networks, net)
	}
}

func createBridgeIface(name string) error {
	kv, err := kernel.GetKernelVersion()
	// only set the bridge's mac address if the kernel version is > 3.3
	// before that it was not supported
	setBridgeMacAddr := err == nil && (kv.Kernel >= 3 && kv.Major >= 3)
	log.Debugf("setting bridge mac address = %v", setBridgeMacAddr)
	return netlink.CreateBridge(name, setBridgeMacAddr)
}

// configureBridge attempts to create and configure a network bridge interface named `ifaceName` on the host
// If bridgeIP is empty, it will try to find a non-conflicting IP from the Docker-specified private ranges
// If the bridge `ifaceName` already exists, it will only perform the IP address association with the existing
// bridge (fixes issue #8444)
// If an address which doesn't conflict with existing interfaces can't be found, an error is returned.
func configureBridge(bridgeIface string, network *net.IPNet) error {
	if err := createBridgeIface(bridgeIface); err != nil {
		// the bridge may already exist, therefore we can ignore an "exists" error
		if !os.IsExist(err) {
			return err
		}
	}
	iface, err := net.InterfaceByName(bridgeIface)
	if err != nil {
		return err
	}
	if netlink.NetworkLinkAddIp(iface, network.IP, network); err != nil {
		return fmt.Errorf("Unable to add private network: %s", err)
	}
	if err := netlink.NetworkLinkUp(iface); err != nil {
		return fmt.Errorf("DefaultBridgeUnable to start network bridge: %s", err)
	}
	return nil
}

func setupIPTables(iface string, addr net.Addr, icc, ipmasq bool) error {
	// Enable NAT

	if ipmasq {
		natArgs := []string{"POSTROUTING", "-t", "nat", "-s", addr.String(), "!", "-o", iface, "-j", "MASQUERADE"}

		if !iptables.Exists(natArgs...) {
			if output, err := iptables.Raw(append([]string{"-I"}, natArgs...)...); err != nil {
				return fmt.Errorf("Unable to enable network bridge NAT: %s", err)
			} else if len(output) != 0 {
				return fmt.Errorf("Error iptables postrouting: %s", output)
			}
		}
	}

	var (
		args       = []string{"FORWARD", "-i", iface, "-o", iface, "-j"}
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
	outgoingArgs := []string{"FORWARD", "-i", iface, "!", "-o", iface, "-j", "ACCEPT"}
	if !iptables.Exists(outgoingArgs...) {
		if output, err := iptables.Raw(append([]string{"-I"}, outgoingArgs...)...); err != nil {
			return fmt.Errorf("Unable to allow outgoing packets: %s", err)
		} else if len(output) != 0 {
			return fmt.Errorf("Error iptables allow outgoing: %s", output)
		}
	}

	// Accept incoming packets for existing connections
	existingArgs := []string{"FORWARD", "-o", iface, "-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT"}

	if !iptables.Exists(existingArgs...) {
		if output, err := iptables.Raw(append([]string{"-I"}, existingArgs...)...); err != nil {
			return fmt.Errorf("Unable to allow incoming packets: %s", err)
		} else if len(output) != 0 {
			return fmt.Errorf("Error iptables allow incoming: %s", output)
		}
	}
	return nil
}
