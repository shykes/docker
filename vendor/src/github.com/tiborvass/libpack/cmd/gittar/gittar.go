package main

import (
	"log"
	"os"

	"github.com/docker/libpack"
)

func main() {
	err := libpack.Git2tar(os.Args[1], os.Args[2], os.Stdout)
	if err != nil {
		log.Fatal(err)
	}
}
