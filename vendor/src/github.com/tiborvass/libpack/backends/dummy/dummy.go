package dummy

import (
	"errors"
	"log"
	"os"

	git "github.com/libgit2/git2go"
)

var ErrNotFinished = errors.New("not finished")

type odb struct {
	log *log.Logger
}

func NewOdbBackend(repo *git.Repository) (git.GoOdbBackend, error) {
	return odb{log: log.New(os.Stdout, "odb:  ", 0)}, nil
}

func (odb odb) Read(oid *git.Oid) ([]byte, git.ObjectType, error) {
	odb.log.Printf("Read: %v", oid)
	return nil, git.ObjectAny, ErrNotFinished
}

func (odb odb) ReadPrefix(shortId []byte) ([]byte, git.ObjectType, *git.Oid, error) {
	odb.log.Printf("ReadPrefix: %v", shortId)
	return nil, git.ObjectAny, nil, ErrNotFinished
}

func (odb odb) ReadHeader(oid *git.Oid) (int, git.ObjectType, error) {
	odb.log.Printf("ReadHeader: %v", oid)
	return 0, git.ObjectAny, ErrNotFinished
}

func (odb odb) Write(oid *git.Oid, buf []byte, oType git.ObjectType) error {
	odb.log.Printf("Write: %v, %v, %v", oid, buf, oType)
	return ErrNotFinished
}

func (odb odb) Exists(oid *git.Oid) bool {
	odb.log.Printf("Exists: %p", oid)
	return false
}

func (odb odb) ExistsPrefix(shortId []byte) (*git.Oid, bool) {
	odb.log.Printf("ExistsPrefix: %v", shortId)
	return nil, false
}

func (odb odb) Refresh() error {
	odb.log.Printf("Refresh")
	return ErrNotFinished
}

func (odb odb) ForEach(cb git.OdbForEachCallback) error {
	odb.log.Printf("ForEach: %v", cb)
	return ErrNotFinished
}

func (odb odb) Free() {
	odb.log.Printf("Free")
}

type refdb struct {
	log  *log.Logger
	repo *git.Repository
}

func NewRefdbBackend(repo *git.Repository, _ *git.Refdb) (git.GoRefdbBackend, error) {
	return refdb{repo: repo, log: log.New(os.Stdout, "refdb:", 0)}, nil
}

func (refdb refdb) Repository() *git.Repository {
	refdb.log.Printf("Repository")
	return refdb.repo
}

func (refdb refdb) Exists(refName string) (bool, error) {
	refdb.log.Printf("Free")
	return false, ErrNotFinished
}

func (refdb refdb) Lookup(refName string) (*git.Reference, error) {
	refdb.log.Printf("Lookup: %v", refName)
	return nil, ErrNotFinished
}

func (refdb refdb) Write(ref *git.Reference, force bool, who *git.Signature, message string, oldId *git.Oid, oldTarget string) error {
	refdb.log.Printf("Write: %v, %v, %v, %v, %v, %v", ref, force, who, message, oldId, oldTarget)
	return ErrNotFinished
}

func (refdb refdb) Rename(oldName, newName string, force bool, who *git.Signature, message string) (*git.Reference, error) {
	refdb.log.Printf("Rename: %v, %v, %v, %v, %v", oldName, newName, force, who, message)
	return nil, ErrNotFinished
}

func (refdb refdb) Del(refName string, oldId *git.Oid, oldTarget string) error {
	refdb.log.Printf("Del: %v, %v, %v", refName, oldId, oldTarget)
	return ErrNotFinished
}

func (refdb refdb) Compress() error {
	refdb.log.Printf("Compress")
	return ErrNotFinished
}

func (refdb refdb) HasLog(refName string) bool {
	refdb.log.Printf("HasLog: %v", refName)
	return false
}

func (refdb refdb) EnsureLog(refName string) error {
	refdb.log.Printf("EnsureLog: %v", refName)
	return ErrNotFinished
}

func (refdb refdb) Free() {
	refdb.log.Printf("Free")
}

func (refdb refdb) Lock(refName string) (interface{}, error) {
	refdb.log.Printf("Lock: %v", refName)
	return nil, ErrNotFinished
}

func (refdb refdb) Unlock(payload interface{}, success, updateReflog bool, ref *git.Reference, sig *git.Signature, message string) error {
	refdb.log.Printf("Unlock: %v, %v, %v, %v, %v, %v", payload, success, updateReflog, ref, sig, message)
	return ErrNotFinished
}
