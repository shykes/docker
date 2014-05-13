package main

import (
	"fmt"
	"github.com/dotcloud/docker/ur"
	"io"
	"log"
	"os"
)

func main() {
	p, err := ur.Compile(os.Stdin)
	if err != nil {
		log.Fatalf("compile: %s", err)
	}
	r := ur.New(ur.DummyId("urc"))
	rs := &ur.Service{nil, r}
	eval, err := rs.Eval(p)
	if err != nil {
		log.Fatalf("eval: %v", err)
	}
	eval.Sender.Close()
	for {
		msg, _, _, err := eval.Receive(0)
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%v\n", msg)
	}
}
