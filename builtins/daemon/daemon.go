package daemon

import (
	"github.com/dotcloud/docker"
	"github.com/dotcloud/docker/engine"
)

func init() {
	engine.Register("daemon", docker.DaemonPlugin)
}

