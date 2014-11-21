package server

import (
	"net/http"

	"github.com/docker/docker/engine"
	"github.com/docker/docker/pkg/version"
)

func postCmd(eng *engine.Engine, version version.Version, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	eng.ServeHTTP(w, r)
	return nil
}
