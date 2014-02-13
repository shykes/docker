package rest

import (
	"github.com/dotcloud/docker/api"
	"github.com/dotcloud/docker/engine"
)

func init() {
	engine.Register("rest", func(job *engine.Job) engine.Status {
		job.Eng.Register("restserver", api.ServeApi)
		return engine.StatusOK
	})
}

