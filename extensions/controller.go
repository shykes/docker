package extensions

import (
	"fmt"

	"github.com/docker/docker/core"
	"github.com/docker/docker/network"
	"github.com/docker/docker/state"
)

// ExtensionController manages the lifetime of loaded extensions. It provides
// an implementation of core.Core for extensions to interact with Docker.
func NewController(state state.State) *Controller {
	return &Controller{
		state:      state,
		extensions: []core.DID{},
	}
}

// NOTE netdriver: all extensions are initialized by calling New...()
// and passing a dedicated state object.
// The extension is responsible for 1) reading initial state for initialization,
// and 2) continue watching for state changes to resolve them. This means
// extensions need a way to spawn long-running goroutines. The core is
// responsible for providing a facility for that.
type Controller struct {
	state      state.State
	extensions []core.DID
}

func (c *Controller) Restore(state state.State) error {
	// Go over all extensions.
	// Re-initialize those that are activated.
	return nil
}

func (c *Controller) Install(name string) error {
	// FIXME: hardcoded extensions should have their own hardcoded ID.
	if id, err := c.Get(name); err == nil {
		return fmt.Errorf("failed to install extension with duplicated ID %q", id)
	}

	/// FIXME: none of this actually works
	//extCore := newCoreProvider(c)
	//if err := extension.Install(extCore); err != nil {
	//return fmt.Errorf("failed to install extension: %v", err)
	//}

	//c.extensions[id] = &extensionData{extension: extension}
	return nil
}

func (c *Controller) Get(name string) (Extension, error) {
	state, err := c.state.Scope("extensions/" + name + "/state")
	if err != nil {
		return nil, err
	}
	return &builtinExtension{c, state}, nil
}

type builtinExtension struct {
	c     *Controller
	state state.State
}

func (e *builtinExtension) newContext() (*builtinContext, error) {
	scope, err := e.state.Scope("/config")
	if err != nil {
		return nil, err
	}

	ctx := &builtinContext{
		c:      e.c,
		e:      e,
		state:  e.state,
		config: scope,
	}

	// FIXME get to compile
	//ctx.Context, ctx.cancel = context.WithCancel(context.Background())
	return ctx, nil
}

func (e *builtinExtension) Install(c Context) error {
	return nil
}

func (ctx *builtinExtension) Uninstall(c Context) error {
	return nil
}

func (ctx *builtinExtension) Disable(c Context) error {
	return nil
}

func (ctx *builtinExtension) Enable(c Context) error {
	return nil
}

// builtinContext exposes the core-facing side of a Context.
type builtinContext struct {
	//context.Context // FIXME get to compile

	e Extension
	c *Controller

	state  state.State
	config state.State
	cancel func()
}

func (ctx *builtinContext) MyState() state.State {
	return ctx.state
}

func (ctx *builtinContext) MyConfig() state.State {
	return ctx.config
}

func (ctx *builtinContext) RegisterNetworkDriver(driver network.Driver, name string) error {
	// FIXME:networking Quick & dirty test code
	// FIXME get to compile first
	//ctx.c.daemon.networks.AddDriver(driver)
	return nil
}

func (c *Controller) Enable(name string) error {
	/* FIXME get to compile
	ext, err := c.getExtensionData(id)
	if err != nil {
		return err
	}

	// Silently ignore is extension is already enabled.
	if ext.enabled {
		log.Debugf("Attempt to Enable() an already enabled extension %q", id)
		return nil
	}

	extCore := newCoreProvider(c)
	if err := ext.extension.Enable(extCore); err != nil {
		return err
	}

	ext.enabled = true
	return nil
	*/
	return nil
}

func (c *Controller) Disable(id core.DID) error {
	/* FIXME get to compile
	ext, err := c.getExtensionData(id)
	if err != nil {
		return err
	}

	// Silently ignore is extension is already disabled.
	if !ext.enabled {
		log.Debugf("Attempt to Disable() an already disabled extension %q", id)
		return nil
	}

	extCore := newCoreProvider(c)
	if err := ext.extension.Disable(extCore); err != nil {
		return err
	}

	ext.enabled = false
	*/
	return nil
}

type extensionData struct {
	enabled bool
}

func (c *Controller) Available() []core.DID {
	return c.listExtensions(func(e *extensionData) bool { return true })
	return []core.DID{}
}

func (c *Controller) Enabled() []core.DID {
	return c.listExtensions(func(e *extensionData) bool { return e.enabled })
}

func (c *Controller) Disabled() []core.DID {
	return c.listExtensions(func(e *extensionData) bool { return !e.enabled })
}

func (c *Controller) listExtensions(predicate func(*extensionData) bool) []core.DID {
	result := make([]core.DID, 0, len(c.extensions))
	for _, did := range c.extensions {
		result = append(result, did)
	}
	return result

	return []core.DID{}
}
