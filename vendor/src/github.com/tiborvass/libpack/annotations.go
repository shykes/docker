package libpack

import (
	"fmt"
	"path"
	"strconv"
	"strings"

	git "github.com/libgit2/git2go"
)

func (db *DB) GetAnnotation(name string) (string, error) {
	return db.Get(MkAnnotation(name))
}

func (db *DB) SetAnnotation(name, value string) error {
	return db.Set(MkAnnotation(name), value)
}

func (db *DB) DeleteAnnotation(name string) error {
	return db.Delete(MkAnnotation(name))
}

func (db *DB) WalkAnnotations(h func(name, value string)) error {
	return db.Walk("/", func(k string, obj git.Object) error {
		blob, isBlob := obj.(*git.Blob)
		if !isBlob {
			return nil
		}
		targetPath, err := ParseAnnotation(k)
		if err != nil {
			return err
		}
		h(targetPath, string(blob.Contents()))
		return nil
	})
}

func MkAnnotation(target string) string {
	target = TreePath(target)
	if target == "/" {
		return "0"
	}
	return fmt.Sprintf("%d/%s", strings.Count(target, "/")+1, target)
}

func ParseAnnotation(annot string) (target string, err error) {
	annot = TreePath(annot)
	parts := strings.Split(annot, "/")
	if len(parts) == 0 {
		return "", fmt.Errorf("invalid annotation path")
	}
	lvl, err := strconv.ParseInt(parts[0], 10, 32)
	if err != nil {
		return "", err
	}

	if int(lvl) == 0 {
		return "", nil
	}

	if len(parts)-1 != int(lvl) {
		return "", fmt.Errorf("invalid annotation path")
	}

	return path.Join(parts[1:]...), nil
}
