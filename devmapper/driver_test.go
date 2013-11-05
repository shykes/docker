package devmapper

import (
	"fmt"
	"io/ioutil"
	"github.com/dotcloud/docker/graphdriver"
	"os"
	"testing"
)

type TestImage struct {
	id	string
	path	string
}

func (img *TestImage) Layers() ([]string, error) {
	return nil, fmt.Errorf("Not implemented")
}

func (img *TestImage) GetID() string {
	return img.id
}

func (img *TestImage) GetParentImage() (graphdriver.Image, error) {
	return nil, nil
}

func mkTestImage(t *testing.T) graphdriver.Image {
	return &TestImage{
		path:	mkTestDirectory(t),
		id:	"4242",
	}
}

func mkTestDirectory(t *testing.T) string {
	dir, err := ioutil.TempDir("", "docker-test-devmapper-")
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestInit(t *testing.T) {
	home := mkTestDirectory(t)
	defer os.RemoveAll(home)
	plugin, err := graphdriver.New("devicemapper")
	if err != nil {
		t.Fatal(err)
	}
	dmplugin := plugin.(*Driver)
	if dmplugin == nil {
		t.Fatal("driver is not devicemapper")
	}
	defer func() {
		return
		if err := dmplugin.Cleanup(); err != nil {
			t.Fatal(err)
		}
	}()
	img := mkTestImage(t)
	defer os.RemoveAll(img.(*TestImage).path)
	if err := dmplugin.OnCreate(img, nil); err != nil {
		t.Fatal(err)
	}
}
