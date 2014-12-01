package main

import (
	"fmt"
	"github.com/codegangsta/cli"
	"os"
	"strings"

	"github.com/docker/libpack/db"
)

const (
	DefaultRef string = "refs/heads/cfg"
)

func main() {
	app := cli.NewApp()
	app.Name = "cfg"
	app.Usage = "A simple command-line interface to git-backed config"
	app.Version = "0.0.1"
	app.Flags = []cli.Flag{}
	app.Commands = []cli.Command{
		{
			Name:   "set",
			Usage:  "",
			Action: cmdSet,
		},
	}
	app.Run(os.Args)
}

func cmdSet(c *cli.Context) {
	if !c.Args().Present() {
		Fatalf("usage: set KEY=VALUE...")
	}
	db, err := db.Init(".git", DefaultRef, "")
	if err != nil {
		Fatalf("init: %v", err)
	}
	for _, arg := range c.Args() {
		parts := strings.SplitN(arg, "=", 2)
		key := parts[0]
		var val string
		if len(parts) >= 2 {
			val = parts[1]
		}
		if err := db.Set(key, val); err != nil {
			Fatalf("set: %v", err)
		}
	}
	if err := db.Commit(fmt.Sprintf("set %s", strings.Join(c.Args(), " "))); err != nil {
		Fatalf("commit: %v", err)
	}
}
func Fatalf(msg string, args ...interface{}) {
	if !strings.HasSuffix(msg, "\n") {
		msg = msg + "\n"
	}
	fmt.Fprintf(os.Stderr, msg, args...)
	os.Exit(1)
}
