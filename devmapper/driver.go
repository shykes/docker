package devmapper

import (
	"fmt"
	"github.com/dotcloud/docker/archive"
	"github.com/dotcloud/docker/changes"
	"github.com/dotcloud/docker/graphdriver"
	"os"
	"path"
)

func init() {
	graphdriver.Register("devicemapper", Init)
}

// End of placeholder interfaces.

type Driver struct {
	*DeviceSet
	home string
}

func Init(home string) (graphdriver.Driver, error) {
	d := &Driver{
		DeviceSet: NewDeviceSet(home),
		home:      home,
	}
	if err := d.DeviceSet.ensureInit(); err != nil {
		return nil, err
	}
	return d, nil
}

func (d *Driver) Cleanup() error {
	return d.DeviceSet.Shutdown()
}

func (d *Driver) OnCreate(img graphdriver.Image, layer archive.Archive) error {
	// Determine the source of the snapshot (parent id or init device)
	var parentID string
	if parent, err := img.GetParentImage(); err != nil {
		return err
	} else if parent != nil {
		parentID = parent.GetID()
	}
	// Create the device for this image by snapshotting source
	if err := d.DeviceSet.AddDevice(img.GetID(), parentID); err != nil {
		return err
	}
	// Mount the device in rootfs
	mp := d.mountpoint(img.GetID())
	if err := os.MkdirAll(mp, 0700); err != nil {
		return err
	}
	if err := d.DeviceSet.MountDevice(img.GetID(), mp, false); err != nil {
		return err
	}
	// Apply the layer as a diff
	if layer != nil {
		if err := archive.ApplyLayer(mp, layer); err != nil {
			return err
		}
	}
	return nil
}

func (d *Driver) OnRemove(img graphdriver.Image) error {
	id := img.GetID()
	if err := d.DeviceSet.RemoveDevice(id); err != nil {
		return fmt.Errorf("Unable to remove device for %v: %v", id, err)
	}
	return nil
}

func (d *Driver) mountpoint(id string) string {
	if d.home == "" {
		return ""
	}
	return path.Join(d.home, "mnt", id)
}

func (d *Driver) Changes(img *graphdriver.Image, dest string) ([]changes.Change, error) {
	return nil, fmt.Errorf("Not implemented")
}

func (d *Driver) Layer(img *graphdriver.Image, dest string) (archive.Archive, error) {
	return nil, fmt.Errorf("Not implemented")
}

func (a *Driver) Mount(img graphdriver.Image, root string) error {
	return fmt.Errorf("Not implemented")
}

func (a *Driver) Unmount(root string) error {
	return fmt.Errorf("Not implemented")
}

func (a *Driver) Mounted(root string) (bool, error) {
	return false, fmt.Errorf("Not implemented")
}
