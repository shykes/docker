package aufs

import (
	"bufio"
	"io/ioutil"
	"os"
	"path"
)

// Return all the directories
func loadIds(root string) ([]string, error) {
	dirs, err := ioutil.ReadDir(root)
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, d := range dirs {
		if !d.IsDir() {
			out = append(out, d.Name())
		}
	}
	return out, nil
}

// Read the layers file for the current id and return all the
// layers represented by new lines in the file
//
// If there are no lines in the file then the id has no parent
// and an empty slice is returned.
func getParentIds(root, id string) ([]string, error) {
	f, err := os.Open(path.Join(root, "layers", id))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := []string{}
	s := bufio.NewScanner(f)

	for s.Scan() {
		if t := s.Text(); t != "" {
			out = append(out, s.Text())
		}
	}
	return out, s.Err()
}

func getLastLayerIds(root string, parents []string) []string {
	for i := 0; i < len(parents); i++ {
		// If this layer had a rootfs, we only return the layers starting here
		rootfs := path.Join(root, "rootfs", parents[i])
		if _, err := os.Lstat(rootfs); err == nil {
			return parents[0 : i+1]
		}
	}
	return parents
}
