package builtins

import (
	"github.com/dotcloud/docker/engine"

	bridge_ "github.com/dotcloud/docker/networkdriver/lxc"
	daemon_ "github.com/dotcloud/docker"
	rest_ "github.com/dotcloud/docker/api"
)

func Register(eng *engine.Engine) {
	rest(eng)

}

// rest: a RESTful api for cross-docker communication
func rest(eng *engine.Engine) {
	eng.Register("rest", func(job *engine.Job) engine.Status {
		job.Eng.Register("restserver", rest_.ServeApi)
		// FIXME: register "restclient"
		return engine.StatusOK
	})
}

// bridge: a default networking backend for Docker on Linux,
// with a shared bridge, 1 veth pair per container,
// and iptables-based NAT.
//
// bridge can be further customized with custom 'networking drivers'.
func bridge(eng *engine.Engine) {
	eng.Register("bridge", bridge_.InitDriver)
	// Override the standard init_networkdriver call
	// FIXME: once plugins can be chained, we should register on a more
	// generic `init` command.
	eng.Register("init_networkdriver", bridge_.InitDriver)
}

// daemon: a default execution and storage backend for Docker on Linux,
// with the following underlying components:
//
// * Pluggable storage drivers including aufs, vfs, lvm and btrfs.
// * Pluggable execution drivers including lxc and chroot.
//
// In practice `daemon` still includes most core Docker components, including:
//
// * The reference registry client implementation
// * Image management
// * The build facility
// * Logging
//
// These components should be broken off into plugins of their own.
//
func daemon(eng *engine.Engine) {
	eng.Register("daemon", daemon_.DaemonPlugin)
	// FIXME: 'initserver' is deprecated but still used by integration tests
	eng.Register("initserver", daemon_.DaemonPlugin)
}
