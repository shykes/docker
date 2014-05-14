package ur

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	beam "github.com/dotcloud/docker/pkg/beam/inmem"
)

func Cli(msg *beam.Message, in beam.Receiver, out beam.Sender) error {
	switch msg.Name {
	case "error":
		{
			fmt.Fprintf(os.Stderr, "===> error: %s\n", strings.Join(msg.Args[:1], ""))
		}
	case "log":
		{
			fmt.Printf("===> %s\n", strings.Join(msg.Args[:1], ""))
		}
	case "prompt":
		{
			key := strings.Join(msg.Args[:1], "")
			fmt.Printf("%s: ", key)
			scanner := bufio.NewScanner(os.Stdin)
			scanner.Scan()
			if err := scanner.Err(); err != io.EOF && err != nil {
				return err
			}
			val := scanner.Text()
			out.Send(&beam.Message{"set", []string{key, val}, nil}, 0)
		}
	default:
		fmt.Fprintf(os.Stderr, "[cli] skipping unknown command '%s'\n", msg.Name)
	}
	return nil
}

func HandlerPipe(h func(*beam.Message, beam.Receiver, beam.Sender) error) beam.Sender {
	hR, hW := beam.Pipe()
	go func() {
		defer hR.Close()
		for {
			msg, msgr, msgw, err := hR.Receive(0)
			if err != nil {
				return
			}
			_ = h(msg, msgr, msgw)
		}
	}()
	return hW
}
