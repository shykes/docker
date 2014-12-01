package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/codegangsta/cli"
	"github.com/docker/docker/archive"
	"github.com/docker/libpack"
)

func main() {
	app := cli.NewApp()
	app.Name = "pack"
	app.Usage = "A simple command-line interface to libpack"
	app.Version = "0.0.1"
	app.Flags = []cli.Flag{}
	app.Commands = []cli.Command{
		{
			Name:   "unpack",
			Usage:  "",
			Action: cmdUnpack,
		},
		{
			Name:   "pack",
			Usage:  "",
			Action: cmdPack,
		},
	}
	app.Run(os.Args)
}

func cmdUnpack(c *cli.Context) {
	if !c.Args().Present() {
		Fatalf("usage: unpack HASH")
	}
	if err := Unpack(".git", ".", c.Args()[0]); err != nil {
		Fatalf("unpack: %v", err)
	}
}

func cmdPack(c *cli.Context) {
	if len(c.Args()) != 1 {
		Fatalf("usage: pack BRANCH")
	}
	hash, err := Pack(".git", ".", c.Args()[0])
	if err != nil {
		Fatalf("pack: %v", err)
	}
	fmt.Println(hash)
}

func Fatalf(msg string, args ...interface{}) {
	if !strings.HasSuffix(msg, "\n") {
		msg = msg + "\n"
	}
	fmt.Fprintf(os.Stderr, msg, args...)
	os.Exit(1)
}

func Pack(repo, dir, branch string) (hash string, err error) {
	db, err := libpack.Init(repo, branch)
	if err != nil {
		return "", err
	}
	a, err := archive.TarWithOptions(dir, &archive.TarOptions{Excludes: []string{".git"}})
	if err != nil {
		return "", err
	}
	if err := db.SetTar(a); err != nil {
		return "", err
	}
	if err := db.Commit("imported tar filesystem tree"); err != nil {
		return "", err
	}
	head := db.Head()
	if head != nil {
		hash = head.String()
	}
	return
}

func Unpack(repo, dir, hash string) error {
	db, err := libpack.Init(repo, hash)
	if err != nil {
		return err
	}
	r, w := io.Pipe()
	var (
		inErr  error
		outErr error
	)
	var tasks sync.WaitGroup
	tasks.Add(2)
	go func() {
		inErr = db.GetTar(os.Stdout)
		w.Close()
		tasks.Done()
	}()
	go func() {
		outErr = archive.Untar(r, dir, &archive.TarOptions{})
		tasks.Done()
	}()
	tasks.Wait()
	if inErr != nil {
		return fmt.Errorf("git2tar: %v", inErr)
	}
	if outErr != nil {
		return fmt.Errorf("untar: %v", outErr)
	}
	return nil
}
