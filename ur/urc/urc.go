package main

import (
	beam "github.com/dotcloud/docker/pkg/beam/inmem"
	"github.com/dotcloud/docker/ur"
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
	if _, err := beam.Copy(ur.HandlerPipe(ur.Cli), eval.Receiver); err != nil {
		log.Fatal(err)
	}
}
