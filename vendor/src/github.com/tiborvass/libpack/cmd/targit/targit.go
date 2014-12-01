package main

import (
	"fmt"
	"log"
	"os"

	"github.com/docker/libpack"
)

func main() {
	result, err := libpack.Tar2git(os.Stdin, os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result)
}
