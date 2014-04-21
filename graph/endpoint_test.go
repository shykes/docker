package graph

import (
	"testing"
	"github.com/dotcloud/docker/engine"
	"github.com/dotcloud/docker/utils"
	"path"
	"os"
)

func TestTagStoreInstall(t *testing.T) {
	eng := tmpEngine(t)
	defer rmEngine(eng)
	tmp, err := utils.TestDirectory("")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)
	s, err := NewTagStore(path.Join(tmp, "store"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Install(eng); err != nil {
		t.Fatal(err)
	}
	if !eng.Exists("getimage") {
		t.Fatalf("%#v\n", *eng)
	}
}


// FIXME: make this reusable across tests
func tmpEngine(t *testing.T) *engine.Engine {
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

func rmEngine(eng *engine.Engine) {
	os.RemoveAll(eng.Root())
}

