package server

import (
	"encoding/json"
	"net/http"

	"github.com/docker/docker/api"
	"github.com/docker/docker/engine"
	"github.com/docker/docker/pkg/version"
)

func postCmd(eng *engine.Engine, version version.Version, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	if err := checkForJson(r); err != nil {
		return err
	}

	cmd := &api.Cmd{}
	if err := json.NewDecoder(r.Body).Decode(cmd); err != nil {
		return err
	}

	job := eng.Job(cmd.Name, cmd.Args...)
	job.Stdout.Add(w)
	// FIXME: @aanand - find out why this makes error responses go wrong
	// job.Stderr.Add(w)

	if err := job.ImportEnv(cmd.Env); err != nil {
		return err
	}

	return job.Run()
}
