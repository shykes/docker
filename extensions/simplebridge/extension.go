package simplebridge

import "github.com/docker/docker/extensions/context"

const driverName = "bridge"

type Extension struct{}

func (e Extension) Install(c context.Context) error {
	return c.RegisterNetworkDriver(&BridgeDriver{state: c.MyState(), schema: NewSchema(c.MyState())}, driverName)
}

func (e Extension) Uninstall(c context.Context) error {
	return c.UnregisterNetworkDriver(driverName)
}

func (e Extension) Enable(c context.Context) error  { return nil }
func (e Extension) Disable(c context.Context) error { return nil }
