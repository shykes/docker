// A dirstore is a directory holding a flat collection of directories,
// each addressable with a unique ID.
//
// This package offers convenience functions to manipulate these directories
// in a reliable and atomic way.
package dirstore

import (
	"fmt"
	"os"
	"path"
	"github.com/dotcloud/docker/gograph"
)

const TRASH_PREFIX string = "_trash_"

// List returns the IDs of all directories currently registered in <store>.
// <store> should be the path of the store on the filesystem.
// Each returned ID is such that path.Join(store, id) is the path to that
// directory on the filesystem.
func List(store string) ([]string, error) {
	db, err := gograph.NewDatabase(path.Join(store, "db"))
	if err != nil {
		return nil, err
	}
	entities := db.List("/", -1)
	dirs := make([]string, 0, len(entities))
	for name, _ := range entities {
		dirs = append(dirs, name)
	}
	return dirs, nil
}

// Create creates a new directory identified as <id> in the store <store>.
// If <store> doesn't exist on the filesystem, it is created.
// If <id> is an empty string, a new unique ID is generated and returned.
func Create(store string, name string) (id string, err error) {
	if err := os.MkdirAll(store, 0700); err != nil {
		return "", err
	}
	var i int64
	// FIXME: store a hint on disk to avoid scanning from 1 everytime
	for i=1; i<1<<63 - 1; i+= 1 {
		id = fmt.Sprintf("%d", i)
		err := os.Mkdir(path.Join(store, id), 0700)
		if os.IsExist(err) {
			continue
		} else if err != nil {
			return "", err
		}
		break
	}
	if i == 1<<63-1 {
		return "", fmt.Errorf("Cant allocate anymore children in %s", store)
	}
	db, err := gograph.NewDatabase(path.Join(store, "db"))
	if err != nil {
		return "", err
	}
	fmt.Printf("Setting %s to %s\n", name, id)
	if _, err := db.Set(path.Join("/", name), id); err != nil {
		os.Remove(path.Join(store, id))
		return "", err
	}
	return id, nil
}

// Trash atomically "trashes" the directory <id> from <store> by
// renaming it to a hidden directory name.
//
// Trash doesn't remove the actual filesystem tree from the store.
// EmptyTrash should be called for that.
func Trash(store string, name string) error {
	db, err := gograph.NewDatabase(path.Join(store, "db"))
	if err != nil {
		return err
	}
	return db.Delete(name)
}

// EmptyTrash scans <store> for directories trashed by Trash(), and
// removes them from the filesystem.
// This is not atomic operation, but it is safe to call it multiple
// times concurrently.
func EmptyTrash(store string) error {
	return fmt.Errorf("Not implemented")
}
