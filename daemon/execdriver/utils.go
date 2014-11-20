package execdriver

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"

	"github.com/docker/docker/pkg/reexec"
	"github.com/docker/docker/utils"
	"github.com/docker/libcontainer/security/capabilities"
)

var nsFlags = map[string]int{
	"net":  syscall.CLONE_NEWNET,
	"ipc":  syscall.CLONE_NEWIPC,
	"ns":   syscall.CLONE_NEWNS,
	"pid":  syscall.CLONE_NEWPID,
	"user": syscall.CLONE_NEWUSER,
	"uts":  syscall.CLONE_NEWUTS,
}

func init() { reexec.Register("docker-createns", createNamespace) }
func createNamespace() {
	runtime.LockOSThread()

	path := flag.String("path", "", "path to bind mount file")
	ns := flag.String("ns", "", "Namespace type")

	flag.Parse()

	flags, ok := nsFlags[*ns]
	if !ok {
		log.Fatalf("Unknown namespace: %s", ns)
	}

	if err := syscall.Unshare(flags); err != nil {
		log.Fatal(err)
	}

	f, err := os.Create(*path)
	if err != nil && !os.IsExist(err) {
		log.Fatal(err)
	}
	if os.IsExist(err) {
		os.Exit(0)
	}
	f.Close()

	if err := syscall.Mount(fmt.Sprintf("/proc/self/ns/%s", *ns), *path, "bind", syscall.MS_BIND, ""); err != nil {
		log.Fatal(err)
	}

	os.Exit(0)
}

func CreateNamespace(ns, path string) error {
	cmd := &exec.Cmd{
		Path: reexec.Self(),
		Args: []string{
			"docker-createns",
			"-ns", ns,
			"-path", path,
		},
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
	return cmd.Run()
}

func TweakCapabilities(basics, adds, drops []string) ([]string, error) {
	var (
		newCaps []string
		allCaps = capabilities.GetAllCapabilities()
	)

	// look for invalid cap in the drop list
	for _, cap := range drops {
		if strings.ToLower(cap) == "all" {
			continue
		}
		if !utils.StringsContainsNoCase(allCaps, cap) {
			return nil, fmt.Errorf("Unknown capability drop: %q", cap)
		}
	}

	// handle --cap-add=all
	if utils.StringsContainsNoCase(adds, "all") {
		basics = capabilities.GetAllCapabilities()
	}

	if !utils.StringsContainsNoCase(drops, "all") {
		for _, cap := range basics {
			// skip `all` aready handled above
			if strings.ToLower(cap) == "all" {
				continue
			}

			// if we don't drop `all`, add back all the non-dropped caps
			if !utils.StringsContainsNoCase(drops, cap) {
				newCaps = append(newCaps, strings.ToUpper(cap))
			}
		}
	}

	for _, cap := range adds {
		// skip `all` aready handled above
		if strings.ToLower(cap) == "all" {
			continue
		}

		if !utils.StringsContainsNoCase(allCaps, cap) {
			return nil, fmt.Errorf("Unknown capability to add: %q", cap)
		}

		// add cap if not already in the list
		if !utils.StringsContainsNoCase(newCaps, cap) {
			newCaps = append(newCaps, strings.ToUpper(cap))
		}
	}

	return newCaps, nil
}
