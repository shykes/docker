// A dirstore is a directory holding a flat collection of directories,
// each addressable with a unique ID.
//
// This package offers convenience functions to manipulate these directories
// in a reliable and atomic way.
package dirstore

import (
	"fmt"
	"os"
	"io/ioutil"
	"path"
	"io"
	"crypto/rand"
	"encoding/hex"
	"strings"
)

const TRASH_PREFIX string = "_trash_"

// List returns the IDs of all directories currently registered in <store>.
// <store> should be the path of the store on the filesystem.
// Each returned ID is such that path.Join(store, id) is the path to that
// directory on the filesystem.
func List(store string) ([]string, error) {
	allDirs, err := listDir(store)
	if err != nil {
		return nil, err
	}
	dirs := make([]string, 0, len(allDirs))
	for _, dir := range allDirs {
		if isHidden(dir) {
			continue
		}
		dirs = append(dirs, dir)
	}
	return dirs, nil
}

// Create creates a new directory identified as <id> in the store <store>.
// If <store> doesn't exist on the filesystem, it is created.
// If <id> is an empty string, a new unique ID is generated and returned.
func Create(store string, id string) (dir string, err error) {
	if err := validateId(id); err != nil {
		return "", err
	}
	if err := os.MkdirAll(store, 0700); err != nil {
		return "", err
	}
	var i int64
	if id != "" {
		if err := os.Mkdir(path.Join(store, id), 0700); err != nil {
			return "", err
		}
		return id, nil
	}
	// FIXME: store a hint on disk to avoid scanning from 1 everytime
	for i=0; i<1<<63 - 1; i+= 1 {
		id = fmt.Sprintf("%d", i)
		err := os.Mkdir(path.Join(store, id), 0700)
		if os.IsExist(err) {
			continue
		} else if err != nil {
			return "", err
		}
		return id, nil
	}
	return "", fmt.Errorf("Cant allocate anymore children in %s", store)
}

// Trash atomically "trashes" the directory <id> from <store> by
// renaming it to a hidden directory name.
//
// Trash doesn't remove the actual filesystem tree from the store.
// EmptyTrash should be called for that.
func Trash(store string, id string) error {
	if err := validateId(id); err != nil {
		return err
	}
	garbageDir := "_trash_" + mkRandomId()
	err := os.Rename(id, garbageDir)
	if err != nil {
		return err
	}
	return nil
}

// EmptyTrash scans <store> for directories trashed by Trash(), and
// removes them from the filesystem.
// This is not atomic operation, but it is safe to call it multiple
// times concurrently.
func EmptyTrash(store string) error {
	dirs, err := listDir(store)
	if err != nil {
		return err
	}
	for _, dir := range dirs {
		if isTrash(dir) {
			if err := os.RemoveAll(path.Join(store, dir)); err != nil {
				return err
			}
		}
	}
	return nil
}

func listDir(store string) ([]string, error) {
	stats, err := ioutil.ReadDir(store)
	if err != nil {
		return nil, err
	}
	dirs := make([]string, 0, len(stats))
	for _, st := range stats {
		if !st.IsDir() {
			continue
		}
		dirs = append(dirs, st.Name())
	}
	return dirs, nil
}


func mkRandomId() string {
	id := make([]byte, 32)
	_, err := io.ReadFull(rand.Reader, id)
	if err != nil {
		panic(err) // This shouldn't happen
	}
	return hex.EncodeToString(id)
}

func validateId(id string) error {
	// Spaces are used as a separator for the suffixarray lookup.
	// FIXME: use a \n separator instead
	if id == "" || isHidden(id) || strings.Contains(id, " ") {
		return fmt.Errorf("Invalid ID: '%s'", id)
	}
	return nil
}

func isHidden(id string) bool {
	return strings.HasPrefix(id, "_")
}

func isTrash(id string) bool {
	return strings.HasPrefix(id, TRASH_PREFIX)
}
