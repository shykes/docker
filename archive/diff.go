package archive

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// ApplyLayer parses a diff in the standard layer format from `layer`, and
// applies it to the directory `dest`.
func ApplyLayer(dest string, layer Archive) error {
	// Poor man's diff applyer in 2 steps:

	// Step 1: untar everything in place
	if err := Untar(layer, dest, nil); err != nil {
		return err
	}

	modifiedDirs := make(map[string]*syscall.Stat_t)
	addDir := func(file string) {
		d := filepath.Dir(file)
		if _, exists := modifiedDirs[d]; !exists {
			if s, err := os.Lstat(d); err == nil {
				if sys := s.Sys(); sys != nil {
					if stat, ok := sys.(*syscall.Stat_t); ok {
						modifiedDirs[d] = stat
					}
				}
			}
		}
	}

	// Step 2: walk for whiteouts and apply them, removing them in the process
	err := filepath.Walk(dest, func(fullPath string, f os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				// This happens in the case of whiteouts in parent dir removing a directory
				// We just ignore it
				return filepath.SkipDir
			}
			return err
		}

		// Rebase path
		path, err := filepath.Rel(dest, fullPath)
		if err != nil {
			return err
		}
		path = filepath.Join("/", path)

		// Skip AUFS metadata
		if matched, err := filepath.Match("/.wh..wh.*", path); err != nil {
			return err
		} else if matched {
			addDir(fullPath)
			if err := os.RemoveAll(fullPath); err != nil {
				return err
			}
		}

		filename := filepath.Base(path)
		if strings.HasPrefix(filename, ".wh.") {
			rmTargetName := filename[len(".wh."):]
			rmTargetPath := filepath.Join(filepath.Dir(fullPath), rmTargetName)

			// Remove the file targeted by the whiteout
			addDir(rmTargetPath)
			if err := os.RemoveAll(rmTargetPath); err != nil {
				return err
			}
			// Remove the whiteout itself
			addDir(fullPath)
			if err := os.RemoveAll(fullPath); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	for k, v := range modifiedDirs {
		aTime := time.Unix(v.Atim.Unix())
		mTime := time.Unix(v.Mtim.Unix())

		if err := os.Chtimes(k, aTime, mTime); err != nil {
			return err
		}
	}

	return nil
}

type TimeUpdate struct {
	path string
	time []syscall.Timeval
	mode uint32
}

// Applies an uncompressed diff layer to a target directory
func ApplyDirLayer(layer, target string, hardLink bool) error {
	var updateTimes []TimeUpdate
	oldmask := syscall.Umask(0)
	defer syscall.Umask(oldmask)
	err := filepath.Walk(layer, func(srcPath string, f os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip root
		if srcPath == layer {
			return nil
		}

		var srcStat syscall.Stat_t
		err = syscall.Lstat(srcPath, &srcStat)
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(layer, srcPath)
		if err != nil {
			return err
		}

		targetPath := filepath.Join(target, relPath)

		// Skip AUFS metadata
		if matched, err := filepath.Match(".wh..wh.*", relPath); err != nil || matched {
			if err != nil || !f.IsDir() {
				return err
			}
			return filepath.SkipDir
		}

		// Find out what kind of modification happened
		file := filepath.Base(srcPath)

		// If there is a whiteout, then the file was removed
		if strings.HasPrefix(file, ".wh.") {
			originalFile := file[len(".wh."):]
			deletePath := filepath.Join(filepath.Dir(targetPath), originalFile)

			err = os.RemoveAll(deletePath)
			if err != nil {
				return err
			}
		} else {
			var targetStat = &syscall.Stat_t{}
			err := syscall.Lstat(targetPath, targetStat)
			if err != nil {
				if !os.IsNotExist(err) {
					return err
				}
				targetStat = nil
			}

			if targetStat != nil && !(targetStat.Mode&syscall.S_IFDIR == syscall.S_IFDIR && srcStat.Mode&syscall.S_IFDIR == syscall.S_IFDIR) {
				// Unless both src and dest are directories we remove the target and recreate it
				// This is a bit wasteful in the case of only a mode change, but that is unlikely
				// to matter much
				err = os.RemoveAll(targetPath)
				if err != nil {
					return err
				}
				targetStat = nil
			}

			if f.IsDir() {
				// Source is a directory
				if targetStat == nil {
					err = syscall.Mkdir(targetPath, srcStat.Mode&07777)
					if err != nil {
						return err
					}
				}
			} else if srcStat.Mode&syscall.S_IFLNK == syscall.S_IFLNK {
				// Source is symlink
				link, err := os.Readlink(srcPath)
				if err != nil {
					return err
				}

				err = os.Symlink(link, targetPath)
				if err != nil {
					return err
				}
			} else if srcStat.Mode&syscall.S_IFBLK == syscall.S_IFBLK ||
				srcStat.Mode&syscall.S_IFCHR == syscall.S_IFCHR ||
				srcStat.Mode&syscall.S_IFIFO == syscall.S_IFIFO ||
				srcStat.Mode&syscall.S_IFSOCK == syscall.S_IFSOCK {
				// Source is special file
				err = syscall.Mknod(targetPath, srcStat.Mode, int(srcStat.Rdev))
				if err != nil {
					return err
				}
			} else if srcStat.Mode&syscall.S_IFREG == syscall.S_IFREG {
				// Source is regular file
				if hardLink {
					if err := os.Link(srcPath, targetPath); err != nil {
						return err
					}
				} else {
					fd, err := syscall.Open(targetPath, syscall.O_CREAT|syscall.O_WRONLY, srcStat.Mode&07777)
					if err != nil {
						return err
					}
					dstFile := os.NewFile(uintptr(fd), targetPath)
					srcFile, err := os.Open(srcPath)
					if err != nil {
						_ = dstFile.Close()
						return err
					}
					_, err = io.Copy(dstFile, srcFile)
					_ = dstFile.Close()
					_ = srcFile.Close()
					if err != nil {
						return err
					}
				}
			} else {
				return fmt.Errorf("Unknown type for file %s", srcPath)
			}

			err = syscall.Lchown(targetPath, int(srcStat.Uid), int(srcStat.Gid))
			if err != nil {
				return err
			}

			if srcStat.Mode&syscall.S_IFLNK != syscall.S_IFLNK {
				err = syscall.Chmod(targetPath, srcStat.Mode&07777)
				if err != nil {
					return err
				}
			}

			ts := []syscall.Timeval{
				syscall.NsecToTimeval(srcStat.Atim.Nano()),
				syscall.NsecToTimeval(srcStat.Mtim.Nano()),
			}

			u := TimeUpdate{
				path: targetPath,
				time: ts,
				mode: srcStat.Mode,
			}

			// Delay time updates until all other changes done, or it is
			// overwritten for directories (by child changes)
			updateTimes = append(updateTimes, u)
		}
		return nil
	})
	if err != nil {
		return err
	}

	// We do this in reverse order so that children are updated before parents
	for i := len(updateTimes) - 1; i >= 0; i-- {
		update := updateTimes[i]

		O_PATH := 010000000 // Not in syscall yet
		var err error
		if update.mode&syscall.S_IFLNK == syscall.S_IFLNK {
			// Update time on the symlink via O_PATH + futimes(), if supported by the kernel

			fd, err := syscall.Open(update.path, syscall.O_RDWR|O_PATH|syscall.O_NOFOLLOW, 0600)
			if err == syscall.EISDIR || err == syscall.ELOOP {
				// O_PATH not supported by kernel, nothing to do, ignore
			} else if err != nil {
				return err
			} else {
				syscall.Futimes(fd, update.time)
				syscall.Close(fd)
			}
		} else {
			err = syscall.Utimes(update.path, update.time)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
