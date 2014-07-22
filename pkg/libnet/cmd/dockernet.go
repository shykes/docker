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
	cfg, err := libgraph.Init(".git", "dockernet/0.0.1", "/")
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
	// Subtree returns the specified subtree, creating it if necessary
	link, err := cfg.Subtree("/scopes", src, "links", name)
	if err != nil {
		Fatalf("%v", err)
	}}
	// Check that the destination exists
	dstSCope, err := cfg.Subtree("/scopes", dst)
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
	//

	if err := allocateIp(cfg, "/ips", path.Join("/scopes", src, "links", "ip")); err != nil {
		t.Fatalf("%v", err)
	}
	cfg.SetBlob(path.Join("/scopes", src, "links", name)

	cfg.Commit(func() error {
		// Conflict resolving handler
		// The only possible conflict is the IP allocation.
		// In case of a conflict

		// Sleep a random backoff period
		// Whoever wakes up first wins (or there will be another conflict to handle)
		time.Sleep(10 * time.Millisecond)
	})

	// Commit the new configuration
	var commitErr error
	for retries:=10; retries--; retries>0 {
		commitErr = cfg.Commit()
		if commitErr == nil {
			return nil
		}

		// FIXME backoff for a random period to decrease chances of collision
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("couldn't commit new configuration: %v", commitErr)
}

func Fatalf(msg string, args ...interface{}) {
	if !strings.HasSuffix(msg, "\n") {
		msg = msg + "\n"
	}
	fmt.Fprintf(os.Stderr, msg, args...)
	os.Exit(1)
}

func allocateIp(cfg *Config, allocPath, dstPath) error {
	allocCfg, err := cfg.Subtree(allocPath)
	if err != nil {
		return err
	}
	ipRange, err := allocCfg.GetBlobDefault("iprange", "")
	if err != nil {
		return err
	}
	if ipRange == "" {
		// FIXME: auto-detect a default network range
		ipRange = "10.42.0.0/16"
	}
	lastIP, err := allocCfg.GetBlobDefault("lastip", "")
	if err != nil {
		return err
	}
	var ip string
	if lastIP == "" {
		// FIXME: compute first IP of the network range
		ip = "42"
	} else {
		// FIXME: increment the lastIP by 1
		// FIXME: add support for released ranges
		var err error
		ip = incrementString(lastIP)
	}
	if err := cfg.SetBlob(dstPath, ip); err != nil {
		return err
	}
	return nil
}

func incrementString(s string) string {
	i, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		i = 0
	}
	return fmt.Sprintf("%d", i+1), nil
}
