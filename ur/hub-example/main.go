package main

import (
	beam "github.com/dotcloud/docker/pkg/beam/inmem"
	"github.com/dotcloud/docker/ur"
	"log"
	"bufio"
	"os"
	"fmt"
	"strings"
	"bytes"
)

func main() {
	cli := ur.NewHub()
	cli.RegisterName("", ur.CliLog)
	cli.RegisterName("print", ur.CliPrint)
	cli.RegisterName("error", ur.CliError)
	cli.RegisterName("log", ur.CliLog)
	rt := ur.NewHub()
	rt.Register(cli)
	rt.RegisterName("eval", ur.RuntimeEval)
	rt.RegisterName("compile", ur.RuntimeCompile)
	input := bufio.NewScanner(os.Stdin)
	for {
		fmt.Printf("> ")
		if !input.Scan() {
			break
		}
		if input.Err() != nil {
			break
		}
		p, err := ur.Compile(strings.NewReader(input.Text()))
		if err != nil {
			fmt.Fprintf(os.Stderr, "compile error: %v\n", err)
			continue
		}
		bc := new(bytes.Buffer)
		p.Encode(bc)
		evalout, _, err := rt.Send(&beam.Message{"eval", []string{bc.String()}}, beam.R)
		if err != nil {
			log.Fatal(err)
		}
		go beam.Copy(cli, evalout)
	}
	rt.Wait()
	cli.Wait()
}
