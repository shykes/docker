package simplebridge

import (
	"net"
	"strconv"

	"github.com/vishvananda/netlink"
)

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

func (d *BridgeDriver) saveNetwork(network string, bridge *BridgeNetwork) error {
	networkSchema := d.schema.Network(network)
	// FIXME allocator, address will be broken if not saved
	if err := networkSchema.Set("bridge_interface", bridge.bridge.Name); err != nil {
		return err
	}

	if err := networkSchema.Set("address", bridge.network.String()); err != nil {
		return err
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
		return nil, err
	}

	addr, err := networkSchema.Get("address")
	if err != nil {
		return nil, err
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
