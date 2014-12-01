package extensions

import (
	"fmt"
	"strings"

	"github.com/tiborvass/libpack"
	"github.com/tiborvass/libpack/backends/dummy"
)

const _DEBUG = true

// GitState uses docker/libpack and satisfies the State interface.
type GitState struct {
	db *libpack.DB
}

// This is backed by libpack/backends/dummy which is a shim and does nothing.
// It is here to show the possibility of having a Go backend for libgit2.
func NewDummyGitState() (*GitState, error) {
	db, err := libpack.OpenWithBackends(libpack.OdbBackendMaker(dummy.NewOdbBackend), libpack.RefdbBackendMaker(dummy.NewRefdbBackend))
	if err != nil {
		return nil, err
	}
	return &GitState{db: db}, nil
}

// GitStateFromFolder returns a ready-to-use GitState.
// The same folder can be used for different stores identified by storeName
// storeName can not contain slashes, as it could be mistaken for a subdirectory.
func GitStateFromFolder(folder, storeName string) (*GitState, error) {
	if strings.Contains(storeName, "/") {
		return nil, fmt.Errorf("Slashes are not allowed in storeName: %q", storeName)
	}
	db, err := libpack.OpenOrInit(folder, "refs/heads/"+storeName)
	if err != nil {
		return &GitState{}, err
	}
	return &GitState{db: db}, nil
}

// Close releases resources for the underlying db.
// Subsequent calls to any methods on the same GitState will result in a panic.
// To recover the same state, call GitStateFromFolder again with the same arguments.
func (s GitState) Close() {
	s.db.Free()
	s.db = nil
}

// Get returns the value associated with `key`.
func (s GitState) Get(key string) (value string, err error) {
	return s.db.Get(key)
}

// List returns a list of keys directly under dir.
// Does not walk the tree recursively.
//
// Example: /
//		foo
//			bar
//		baz
// List("/") -> ["foo", "bar"]
func (s GitState) List(dir string) ([]string, error) {
	res, err := s.db.List(dir)
	if _DEBUG {
		fmt.Printf("List(%q) -> (%v, err:%v)\n", dir, res, err)
	}
	return res, err
}

// Set sets the key `key`, to a value `value`.
// It automatically overrides the existing value if any.
func (s GitState) Set(key, value string) error {
	err := s.db.Set(key, value)
	if err == nil {
		err = s.db.Commit(fmt.Sprintf("set %q=%q", key, value))
	}
	if _DEBUG {
		fmt.Printf("Set(%q, %q) -> err:%v\n", key, value, err)
	}
	return err
}

// Remove deletes the value associated with `key`.
func (s GitState) Remove(key string) error {
	err := s.db.Delete(key)
	if err == nil {
		err = s.db.Commit(fmt.Sprintf("del %q", key))
	}
	if _DEBUG {
		fmt.Printf("Remove(%q) -> err:%v\n", key, err)
	}
	return err
}

// Mkdir creates the directory `dir`.
func (s GitState) Mkdir(dir string) error {
	err := s.db.Mkdir(dir)
	if err == nil {
		err = s.db.Commit(fmt.Sprintf("mkdir %q", dir))
	}
	if _DEBUG {
		fmt.Printf("Mkdir(%q) -> err:%v\n", dir, err)
	}
	return err
}
