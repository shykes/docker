package libpack

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path"

	git "github.com/libgit2/git2go"

	"github.com/docker/docker/vendor/src/code.google.com/p/go/src/pkg/archive/tar"
)

const (
	MetaTree = "_fs_meta"
	DataTree = "_fs_data"
)

// GetTar generates a tar stream frmo the contents of db, and streams
// it to `dst`.
func (db *DB) GetTar(dst io.Writer) error {
	tw := tar.NewWriter(dst)
	defer tw.Close()
	// Walk the data tree
	return db.Walk(DataTree, func(name string, obj git.Object) error {
		fmt.Fprintf(os.Stderr, "Generating tar entry for '%s'...\n", name)
		metaBlob, err := db.Get(metaPath(name))
		if err != nil {
			return err
		}
		tr := tar.NewReader(bytes.NewReader([]byte(metaBlob)))
		hdr, err := tr.Next()
		if err != nil {
			return err
		}
		// Write the reconstituted tar header+content
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if blob, isBlob := obj.(*git.Blob); isBlob {
			fmt.Fprintf(os.Stderr, "--> writing %d bytes for blob %s\n", hdr.Size, hdr.Name)
			if _, err := tw.Write(blob.Contents()[:hdr.Size]); err != nil {
				return err
			}
		}
		return nil
	})
	return nil
}

// SetTar adds data to db from a tar strema decoded from `src`.
// Raw data is stored at the key `_fs_data/', and metadata in a
// separate key '_fs_metadata'.
func (db *DB) SetTar(src io.Reader) error {
	tr := tar.NewReader(src)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		fmt.Printf("[META] %s\n", hdr.Name)
		metaBlob, err := headerReader(hdr)
		if err != nil {
			return err
		}
		fmt.Printf("    ---> storing metadata in %s\n", metaPath(hdr.Name))
		if err := db.SetStream(metaPath(hdr.Name), metaBlob); err != nil {
			continue
		}
		// FIXME: git can carry symlinks as well
		if hdr.Typeflag == tar.TypeReg {
			fmt.Printf("[DATA] %s %d bytes\n", hdr.Name, hdr.Size)
			if err := db.SetStream(path.Join("_fs_data", hdr.Name), tr); err != nil {
				continue
			}
		}
	}
	return nil
}

// metaPath computes a path at which the metadata can be stored for a given path.
// For example if `name` is "/etc/resolv.conf", the corresponding metapath is
// "_fs_meta/194c1cbe5a8cfcb85c6a46b936da12ffdc32f90f"
// This path will be used to store and retrieve the tar header encoding the metadata
// for the corresponding file.
func metaPath(name string) string {
	return path.Join(MetaTree, MkAnnotation(name))
}

func headerReader(hdr *tar.Header) (io.Reader, error) {
	var buf bytes.Buffer
	w := tar.NewWriter(&buf)
	defer w.Close()
	if err := w.WriteHeader(hdr); err != nil {
		return nil, err
	}
	return &buf, nil
}
