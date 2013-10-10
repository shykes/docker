// Test program for dirstore + gograph
// Solomon Hykes <solomon@dotcloud.com>

package main

import (
	"flag"
	"log"
	"fmt"
	"strings"
	"github.com/dotcloud/docker/dirstore"
)

func main() {
	flag.Parse()
	if flag.Arg(0) == "ls" {
		dirs, err := dirstore.List(".")
		if err != nil {
			log.Fatal(err)
		}
		if len(dirs) > 0 {
			fmt.Printf("%s\n", strings.Join(dirs, "\n"))
		}
	} else if flag.Arg(0) == "mkdir" {
		if _, err := dirstore.Create(".", flag.Arg(1)); err != nil {
			log.Fatal(err)
		}
	}
}
