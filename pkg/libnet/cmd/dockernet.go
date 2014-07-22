package main

import (
	"fmt"
	"github.com/codegangsta/cli"
	"os"
	"strings"
)

func main() {
	// Usage examples:
	//
	// dockernet list [<scope>]
	// dockernet connect <scope>/<name> <target>
	// dockernet connect myapp/db mydb
	// dockernet disconnect myapp/db
	// dockernet sync
	// dockernet watch <scope>
	// dockernet query <scope> <filter>
	// dockernet open <scope> [<portspec>...]
	// dockernet close <scope> [<portspec>...]

	app := cli.NewApp()
	app.Name = "dockernet"
	app.Usage = "Experimental tool for Docker networking"
	app.Version = "0.0.1"
	app.Flags = []cli.Flag{}
	app.Commands = []cli.Command{
		{
			Name:	"list",
			Usage:	"",
			Action:	cmdList,
		},
	}
	app.Run(os.Args)
}

func cmdList(c *cli.Context) {
	cfg, err := InitConfig(".git", "dockernet/0.0.1", "/")
	if err != nil {
		Fatalf("%v", err)
	}
	if len(c.Args()) != 1 {
		Fatalf("usage: dockernet list <scope>")
	}
	scope := c.Args()[0]
	links, err := cfg.List(path.Join("/scopes", scope, "links"))
	if err != nil {
		Fatalf("%v", err)
	}
	for _, name := range(links) {
		blob, err := cfg.GetBlob(path.Join("/scopes", scope, "links", name))
		if err != nil {
			Fatalf("%v", err)
		}
		fmt.Printf("%s\t%s\n", name, strings.Replace(blob, "\n", " "))
	}
}

func cmdConnect(c  *cli.Context) {
	cfg, err := InitConfig(".git", "dockernet/0.0.1", "/")
	if err != nil {
		Fatalf("%v", err)
	}
	if len(c.Args()) != 3 {
		Fatalf("usage: dockernet connect <src> <name> <dst>")
	}
	var (
		src = c.Args()[0]
		name = c.Args()[1]
		dst = c.Args()[2]
	)
	snap, err := cfg.Snapshot()
	if err != nil {
		fatalf("%v", err)
	}
	// Subtree returns the specified subtree, creating it if necessary
	link, err := snap.Subtree("/scopes", src, "links", name)
	if err != nil {
		Fatalf("%v", err)
	}}
	// Check that the destination exists
	dstSCope, err := snap.Subtree("/scopes", dst)
	if err != nil {
		Fatalf("no such scope: %s", dst)
	}
	// Set the destination
	if err := link.SetBlob("dst", dst); err != nil {
		fatalf("%v", err)
	}
	// The Docker networking model guarantees that the IP for a given link
	// will not change once set. Therefore we persist IPs in the config.
	//
	// If the IP for this link doesn't exist, allocate one.
	// Note: this requires a separate synchronized change to the config,
	// to guarantee uniqueness of the IP.
	var lastIP string
	if snap.Exists("lastip") {
		var err error
		lastIP, err = snap.GetBlob("lastIP")
		if err != nil {
			Fatalf("%v", err)
		}
	}
	if snap.Exists("bridge
		// FIXME: initialize allocator from bridge config (IP range etc)
		// FIXME: Is Config{} a copy-on-write tree, with 1) a git-backed layer and 2) an inmem layer?
	ipAllocator, err := NewIPAllocator(

		}
		m["dst"] = dst
	} else {
		
	}
	if entry.IsMap() {
		entry.Map().
	}
	val, err := snap.GetMap(path.Join("
	newcfg.SetBlob(path.Join("/scopes", src, "links", name), 
}

func Fatalf(msg string, args ...interface{}) {
	if !strings.HasSuffix(msg, "\n") {
		msg = msg + "\n"
	}
	fmt.Fprintf(os.Stderr, msg, args...)
	os.Exit(1)
}

type Config struct {
	repo *git.Repo
	branch string	// The branch name
	subtree string	// A path relative to t, under which the config is scoped
	t *git.Commit   // The current snapshot
}

func (j *Config) Snapshot(hash string) (*Config, error) {

}

func (j *Config) Get(hash string) (*Tree, error) {

}

func (j *Config) Commit(desc []string, t *Tree) (string, error) {

}
