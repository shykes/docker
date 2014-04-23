package testutils

import (
	"github.com/dotcloud/docker/engine"
	"github.com/dotcloud/docker/utils"
	"testing"
)

// TmpEngine creates a temporary directory and returns an Engine
// using it as a root.
// If an error occurs, t.Fatal is called.
// This is intended for use as a convenience when creating
// mock engines as part of a test suite.
// The caller is responsible for calling Nuke to destroy the
// directory.
func TmpEngine(t *testing.T) *engine.Engine {
	// FIXME: this is only needed because Engine requires a root
	// argument. That argument is not needed anymore, so it should
	// be removed and Tmp along with it.
	tmp, err := utils.TestDirectory("")
	if err != nil {
		t.Fatal(err)
	}
	eng, err := engine.New(tmp)
	if err != nil {
		t.Fatal(err)
	}
	return eng
}
