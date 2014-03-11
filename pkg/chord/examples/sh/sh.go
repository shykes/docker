package main

import (
	"fmt"
	"github.com/dotcloud/docker/pkg/chord"
	"github.com/dotcloud/docker/utils"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path"
	"sync"
	"bufio"
	"strings"
	"io"
)

var commands = map[string]func([]string, *net.UnixConn, *net.UnixConn)error {
	"echo": CmdEcho,
	"dump": CmdDump,
	"listen": CmdListen,
}


func CmdEcho(cmd []string, in *net.UnixConn, out *net.UnixConn) error {
	fmt.Printf("%s\n", strings.Join(cmd[1:], " "))
	return nil
}

func CmdDump(cmd []string, in *net.UnixConn, out *net.UnixConn) error {
	prefix := strings.Join(cmd[1:], " ")
	var wg sync.WaitGroup
	for {
		msg, f, err := chord.Receive(in)
		if err != nil {
			return err
		}
		wg.Add(1)
		go func(src io.Reader) {
			defer wg.Done()
			input := bufio.NewScanner(f)
			for input.Scan() {
				line := input.Text()
				if len(line) > 0 {
					fmt.Printf("%s [%s] %s\n", prefix, msg, line)
				}
				if err := input.Err(); err != nil {
					fmt.Printf("%s [%s:%s]\n", prefix, msg, err)
					break
				}
			}
		}(f)
	}
	wg.Wait()
	return nil
}

func CmdListen(cmd []string, in *net.UnixConn, out *net.UnixConn) error {
	Logf("CmdListen\n")
	addr, err := url.Parse(cmd[1])
	if err != nil {
		return err
	}
	l, err := net.Listen(addr.Scheme, addr.Host)
	if err != nil {
		return err
	}
	defer l.Close()
	for {
		Logf("Listening on %s\n", cmd[1])
		conn, err := l.Accept()
		if err != nil {
			break
		}
		Logf("New connection: %s\n", conn.RemoteAddr())
		fconn, ok := conn.(interface { File() (*os.File, error) })
		if !ok {
			conn.Close()
			continue
		}
		// FIXME: we may be duplicating the fd instead of getting the real one.
		// Does this mean the original fd is leaked? close conn() all the time
		// to fix?
		f, err := fconn.File()
		if err != nil {
			conn.Close()
			continue
		}
		if err := chord.Send(out, []byte("conn"), f); err != nil {
			return err
		}
	}
	return nil
}



const DefaultName = "sh"

// TODO serialize every "root" command with a name

func main() {
	Logf("main")
	var err error
	// This is a hacky workaround, because the high-level Go fork/exec interface
	// doesn't seem to allow setting an arbitrary argv[0].
	// to an arbitrary value.
	if path.Base(os.Args[0]) == DefaultName {
		// ARGV[0] is the default -> SHELL
		//     - read commands from cli and/or stdin
		//     - for each command, fork-exec self with corresponding argv
		//     - each command gets its BEAMIN and BEAMOUT wired based on tree description
		//     - outbound requests are handled by a defaut handler which prints to stdout
		err = doShell()
	} else {
		// ARGV[0] other than default -> BUILTIN
		//     - open BEAMIN and BEAMOUT
		//     - call in-memory builtin with argv as arguments
		//
		err = doChild()
	}
	if err != nil {
		Fatal(err)
	}
}

func doShell() error {
	commands := make(chan []string)
	go func() {
		defer close(commands)
		if len(os.Args) > 1 {
			// Batch mode
			commands <- os.Args[1:]
			return
		}
		// Interactive mode
		input := bufio.NewScanner(os.Stdin)
		for input.Scan() {
			line := input.Text()
			words := strings.Split(line, " ")
			if len(words) > 0 {
				commands <- words
			}
			if input.Err() != nil {
				return
			}
		}
	}()
	var (
		in *os.File
		out *os.File
		tasks sync.WaitGroup
	)
	for args := range commands {
		cmd := Command(utils.SelfPath(), args...)
		cmd.Args = cmd.Args[1:]
		Logf("preparing command '%s'", cmd.Args[0])
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		// make it flow like pipelines (app to the left, plugin to the right)
		// listen tcp://:4242 | foreground
		// First command, setup in pipe
		if in == nil {
			in, err := cmd.InPipe()
			if err != nil {
				return err
			}
			defer in.Close()
		} else {
			cmd.In = out
		}
		// Always setup out pipe
		out, err := cmd.OutPipe()
		if err != nil {
			return err
		}
		defer out.Close()
		cmd.Start()
		tasks.Add(1)
		go func(cmd *Cmd) {
			cmd.Wait()
			Logf("wait: %s\n", cmd.Args[0])
			tasks.Done()
		}(cmd)
	}
	// End the pipeline with a discard drain (to allow the last command to send on out without blocking)
	go func() {
		Logf("draining output\n")
		uout, err := chord.FdConn(int(out.Fd()))
		if err != nil {
			return
		}
		for {
			msg, f, err := chord.Receive(uout)
			if err != nil {
				return
			}
			Logf("drain: %s\n", msg)
			if f != nil {
				go io.Copy(ioutil.Discard, f)
			}
		}
	}()
	tasks.Wait()
	return nil
}

func doChild() error {
	in, err := chord.FdConn(3)
	if err != nil {
		Fatal(err)
	}
	out, err := chord.FdConn(4)
	if err != nil {
		Fatal(err)
	}
	if cmd, exists := commands[os.Args[0]]; exists {
		return cmd(os.Args, in, out)
	}
	return fmt.Errorf("no such command: %s", os.Args[0])
}

func Command(name string, args ...string) *Cmd {
	c := exec.Command(name, args...)
	return &Cmd{
		Cmd: *c,
	}
}

type Cmd struct {
	exec.Cmd

	// We store In and Out as file descriptors
	// because UnixConn doesn't expose the underlying fd
	// UnixConn.File() returns a copy.
	In	*os.File
	Out	*os.File
}

func (cmd *Cmd) InPipe() (*os.File, error) {
	local, remote, err := chord.SocketPair()
	if err != nil {
		return nil, err
	}
	cmd.In = remote
	return local, nil
}

func (cmd *Cmd) OutPipe() (*os.File, error) {
	local, remote, err := chord.SocketPair()
	if err != nil {
		return nil, err
	}
	cmd.Out = remote
	return local, nil
}

func (cmd *Cmd) Run() error {
	// Setup FD=3 (IN)
	if len(cmd.ExtraFiles) == 0 {
		cmd.ExtraFiles = append(cmd.ExtraFiles, cmd.In)
	} else {
		cmd.ExtraFiles[0] = cmd.In
	}
	// Setup FD=4 (OUT)
	if len(cmd.ExtraFiles) == 1 {
		cmd.ExtraFiles = append(cmd.ExtraFiles, cmd.Out)
	} else {
		cmd.ExtraFiles[1] = cmd.In
	}
	Logf("extrafiles: %v\n", cmd.ExtraFiles)
	return cmd.Cmd.Run()
}

func Logf(msg string, args ...interface{}) (int, error) {
	if len(msg) == 0 || msg[len(msg) - 1] != '\n' {
		msg = msg + "\n"
	}
	msg = fmt.Sprintf("[%v] [%v] %s", os.Getpid(), path.Base(os.Args[0]), msg)
	return fmt.Printf(msg, args...)
}

func Fatalf(msg string, args ...interface{})  {
	Logf(msg, args)
	os.Exit(1)
}

func Fatal(args ...interface{}) {
	Fatalf("%v", args[0])
}
