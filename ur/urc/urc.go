package main

import (
	"fmt"
	"github.com/dotcloud/docker/ur"
	"log"
	"os"
)

func main() {
	p, err := ur.Compile(os.Stdin)
	if err != nil {
		log.Fatalf("compile: %s", err)
	}
	n, err := p.Encode(os.Stdout)
	if err != nil {
		log.Fatalf("encode: %s", err)
	}
	fmt.Fprintf(os.Stderr, "Encoded %d instructions\n", n)
}
