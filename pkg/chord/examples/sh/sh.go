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
	"strings"
	"io"
)

func CmdEcho(cmd []string, in *chord.ServerSession, out *chord.Client) error {
	fmt.Printf("%s\n", strings.Join(cmd[1:], " "))
	return nil
}

func CmdListen(cmd []string, in *chord.ServerSession, out *chord.Client) error {
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
		if err := chord.Send(out.Conn(), []byte("conn"), f); err != nil {
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
	if len(os.Args) == 1 {
		return fmt.Errorf("usage: %s COMMAND [ARGS]", os.Args[0])
	}
	cmd := Command(utils.SelfPath(), os.Args[1:]...)
	cmd.Args = cmd.Args[1:]
	Logf("preparing command '%s'", cmd.Args[0])
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	childOut, err := cmd.OutPipe()
	if err != nil {
		return err
	}
	childIn, err := cmd.InPipe()
	if err != nil {
		return err
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer Logf("done listening for output messages from '%s'", cmd.Args[0])
		defer wg.Done()
		for {
			Logf("listening for output messages from '%s'", cmd.Args[0])
			data, conn, err := chord.Receive(childOut)
			Logf("---> from [%v]: '%s' (err=%v)", cmd.Args[0], data, err)
			if err != nil {
				return
			}
			Logf("out from [%s]: '%s'\n", cmd.Args[0], data)
			if conn != nil {
				io.Copy(os.Stdout, conn)
				conn.Close()
			}
		}
	}()
	Logf("Running child path=%v args=%v", cmd.Path, cmd.Args)
	err = cmd.Run()
	Logf("Child returned: err=%v", err)
	childOut.Close()
	childIn.Close()
	Logf("Waiting for output handling goroutine to complete")
	wg.Wait()
	Logf("handling goroutine is complete")
	return err
}

func doChild() error {
	in, err := chord.NewServerSession(nil)
	if err != nil {
		Fatal(err)
	}
	out, err := chord.NewClient(nil)
	if err != nil {
		Fatal(err)
	}
	if os.Args[0] == "listen" {
		return CmdListen(os.Args, in, out)
	} else if os.Args[0] == "echo" {
		return CmdEcho(os.Args, in, out)
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

func (cmd *Cmd) beamPair() (*net.UnixConn, *os.File, error) {
	pair, err := chord.SocketPair()
	if err != nil {
		return nil, nil, err
	}
	local, err := chord.FdConn(pair[1])
	if err != nil {
		return nil, nil, err
	}
	remote := os.NewFile(uintptr(pair[0]), "")
	return local, remote, nil
}

func (cmd *Cmd) InPipe() (*net.UnixConn, error) {
	local, remote, err := cmd.beamPair()
	if err != nil {
		return nil, err
	}
	cmd.In = remote
	return local, nil
}

func (cmd *Cmd) OutPipe() (*net.UnixConn, error) {
	local, remote, err := cmd.beamPair()
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
