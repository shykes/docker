package simplebridge

import "path"

func (d *BridgeDriver) joinNetwork(network string) string {
	return path.Join(NetworkPrefix, network)
}

func (d *BridgeDriver) joinNetworkItem(network, item string) string {
	return path.Join(d.joinNetwork(network), item)
}

func (d *BridgeDriver) joinEndpointItem(network, endpoint, item string) string {
	return path.Join(d.joinEndpoint(network, endpoint), item)
}

func (d *BridgeDriver) joinEndpoint(network, endpoint string) string {
	return path.Join(EndpointPrefix, network, endpoint)
}

func (d *BridgeDriver) createEndpoint(network, endpoint string) error {
	return d.state.Mkdir(d.joinEndpoint(network, endpoint))
}

func (d *BridgeDriver) removeEndpoint(network, endpoint string) error {
	return d.state.Remove(d.joinEndpoint(network, endpoint))
}

func (d *BridgeDriver) endpointExists(network, endpoint string) bool {
	_, err := d.state.Get(d.joinEndpoint(network, endpoint))
	return err == nil
}

func (d *BridgeDriver) getEndpointProperty(network, endpoint, item string) (string, error) {
	return d.state.Get(d.joinEndpointItem(network, endpoint, item))
}

func (d *BridgeDriver) setEndpointProperty(network, endpoint, item, value string) error {
	return d.state.Set(d.joinEndpointItem(network, endpoint, item), value)
}

func (d *BridgeDriver) createNetwork(network string) error {
	return d.state.Mkdir(d.joinNetwork(network))
}

func (d *BridgeDriver) removeNetwork(network string) error {
	return d.state.Remove(d.joinNetwork(network))
}

func (d *BridgeDriver) networkExists(network string) bool {
	_, err := d.state.Get(d.joinNetwork(network))
	return err == nil
}

func (d *BridgeDriver) getNetworkProperty(network, item string) (string, error) {
	return d.state.Get(d.joinNetworkItem(network, item))
}

func (d *BridgeDriver) setNetworkProperty(network, item, value string) error {
	return d.state.Set(d.joinNetworkItem(network, item), value)
}
