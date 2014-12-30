package simplebridge

import (
	"fmt"
	"net"
	"strconv"

	"github.com/vishvananda/netlink"
)

func (d *BridgeDriver) loadEndpoint(network, endpoint string) (*BridgeEndpoint, error) {
	scope := d.schema.Endpoint(network, endpoint)

	iface, err := scope.Get("interface_name")
	if err != nil {
		return nil, loadEndpointError(endpoint, network, err)
	}

	hwAddr, err := scope.Get("hwaddr")
	if err != nil {
		return nil, loadEndpointError(endpoint, network, err)
	}

	mtu, err := scope.Get("mtu")
	if err != nil {
		return nil, loadEndpointError(endpoint, network, err)
	}

	ipaddr, err := scope.Get("ip")
	if err != nil {
		return nil, loadEndpointError(endpoint, network, err)
	}

	ip := net.ParseIP(ipaddr)

	mtuInt, _ := strconv.ParseUint(mtu, 10, 32)

	netObj, err := d.loadNetwork(network)
	if err != nil {
		return nil, loadEndpointError(endpoint, network, err)
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

	if err := scope.MultiSet(pathMap); err != nil {
		return saveEndpointError(ep.ID, network, err)
	}

	return nil
}

func (d *BridgeDriver) saveNetwork(network string, bridge *BridgeNetwork) error {
	networkSchema := d.schema.Network(network)
	// FIXME allocator, address will be broken if not saved
	if err := networkSchema.Set("bridge_interface", bridge.bridge.Name); err != nil {
		return loadNetworkError(network, err)
	}

	if err := networkSchema.Set("address", bridge.network.String()); err != nil {
		return loadNetworkError(network, err)
	}

	if bridge.vxlan != nil {
		networkSchema.Set("vxlan_device", bridge.vxlan.Attrs().Name)
	}

	return nil
}

func (d *BridgeDriver) loadNetwork(network string) (*BridgeNetwork, error) {
	networkSchema := d.schema.Network(network)

	iface, err := networkSchema.Get("bridge_interface")
	if err != nil {
		return nil, loadNetworkError(network, err)
	}

	addr, err := networkSchema.Get("address")
	if err != nil {
		return nil, loadNetworkError(network, err)
	}

	ip, ipNet, err := net.ParseCIDR(addr)
	ipNet.IP = ip

	var vxlan *netlink.Vxlan

	vxdev, err := networkSchema.Get("vxlan_device")
	if err == nil && vxdev != "" {
		vxlan = &netlink.Vxlan{LinkAttrs: netlink.LinkAttrs{Name: vxdev}}
	}

	return &BridgeNetwork{
		vxlan:       vxlan,
		bridge:      &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: iface}},
		ID:          network,
		driver:      d,
		network:     ipNet,
		ipallocator: NewIPAllocator(iface, ipNet, nil, nil),
	}, nil
}

func endpointError(function, ep, n string, err error) error {
	return fmt.Errorf("Error %sing Endpoint %q for Network %q: %v", function, ep, n, err)
}

func networkError(function, n string, err error) error {
	return fmt.Errorf("Error %sing Network %q: %v", function, n, err)
}

func loadEndpointError(ep, n string, err error) error {
	return endpointError("load", ep, n, err)
}

func saveEndpointError(ep, n string, err error) error {
	return endpointError("save", ep, n, err)
}

func loadNetworkError(n string, err error) error {
	return networkError("load", n, err)
}

func saveNetworkError(n string, err error) error {
	return networkError("save", n, err)
}
