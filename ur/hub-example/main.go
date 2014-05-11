package main

import (
	beam "github.com/dotcloud/docker/pkg/beam/inmem"
	"github.com/dotcloud/docker/ur"
	"fmt"
	"log"
	"bufio"
	"os"
)

func main() {
	/*
	p, err := ur.Compile(os.Stdin)
	if err != nil {
		log.Fatalf("compile: %s", err)
	}
	*/
	hub := ur.NewHub()
	// Register handler
	r, w, err := hub.Send(&beam.Message{"register", nil}, beam.R|beam.W)
	if err != nil {
		log.Fatal(err)
	}
	go func() {
		defer w.Close()
		for {
			msg, _, _, err := r.Receive(0)
			if err != nil {
				log.Fatalf("recv: %v", err)
			}
			fmt.Printf("===> %s %s\n", msg.Name, msg.Args)
			if _, _, err := w.Send(msg, 0); err != nil {
				log.Fatalf("send: %v", err)
			}
		}
	}()
	input := bufio.NewScanner(os.Stdin)
	for input.Scan() {
		if input.Err() != nil {
			break
		}
		hub.Send(&beam.Message{"log", []string{input.Text()}}, 0)
	}
}
