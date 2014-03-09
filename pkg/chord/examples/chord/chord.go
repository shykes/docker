package main

import (
	"fmt"
	"github.com/dotcloud/docker/pkg/chord"
	"github.com/dotcloud/docker/utils"
	"io"
	"log"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"path"
)

func forkWorker() (*exec.Cmd, *net.UnixConn, error) {
	child := exec.Command(utils.SelfPath())
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	pair, err := chord.SocketPair()
	if err != nil {
		return nil, nil, err
	}
	remote := os.NewFile(uintptr(pair[0]), "")
	local, err := chord.FdConn(pair[1])
	child.ExtraFiles = append(child.ExtraFiles, remote)
	child.Env = append(child.Env, "CHORD=3")
	return child, local, nil

}

func main() {
	fmt.Printf("[%v] %s\n", os.Getpid(), strings.Join(os.Args, " "))
	if len(os.Args) == 1 {
		if err := worker(); err != nil {
			log.Fatalf("worker: %v\n", err)
		}
	} else {
		fmt.Printf("// 1. fork new worker\n")
		// 1. fork new worker
		child, childHandle, err := forkWorker()
		if err != nil {
			log.Fatal(err)
		}
		var wg sync.WaitGroup
		wg.Add(3)
		go func() {
			defer wg.Done()
			fmt.Printf("Running %s %v\n", child.Path, child.Args)
			err := child.Run()
			fmt.Printf("---> child %d terminated. err=%v\n", child.ProcessState.Pid(), err)
		}()
		// 2. Send bootstrap job to worker
		fmt.Printf("// 2. Send bootstrap job to worker\n")
		go func() {
			defer wg.Done()
			childClient, err := chord.NewClient(childHandle)
			if err != nil {
				log.Fatal(err)
			}
			job, err := childClient.Open(strings.Join(os.Args[1:], " "))
			if err != nil {
				log.Fatal(err)
			}
			for {
				name, conn, err := job.Receive()
				if err != nil {
					break
				}
				fmt.Printf("---> [%s]\n", name)
				var attachgroup sync.WaitGroup
				attachgroup.Add(1)
				go func() {
					io.Copy(os.Stdout, conn)
					attachgroup.Done()
				}()
				go func() {
					io.Copy(conn, os.Stdin)
					attachgroup.Done()
				}()
				attachgroup.Wait()
				fmt.Printf("---> [%s] done\n", name)
			}
		}()
		// 3. Serve callbacks from worker
		fmt.Printf("// 3. Serve callbacks from worker\n")
		go func() {
			defer wg.Done()
			srv, err := chord.NewServerSession(childHandle)
			if err != nil {
				return
			}
			srv.Serve(func(data []byte, conn *net.UnixConn) {
				fmt.Printf("worker [%s] calling back for [%s]\n", os.Args[1], string(data))
			})
		}()
		wg.Wait()
	}
}

func worker() error {
	fmt.Printf("[%v] initializing worker\n", os.Getpid())
	c, err := chord.NewClient(nil)
	if err != nil {
		return err
	}
	conn := c.Conn()
	srv, err := chord.NewServerSession(conn)
	if err != nil {
		return err
	}
	Logf("Waiting for job requests on %v\n", conn)
	srv.ServeSimple(func(data string, out chan *os.File) {
		cmd := strings.Split(data, " ")
		if cmd[0] == "listen" {
			CmdListen(cmd, out)
		} else if cmd[0] == "open" {
			CmdOpen(cmd, out)
		}
	})
	return nil
}


func CmdOpen(cmd []string, out chan *os.File) {
	f, err := os.Open(cmd[1])
	if err != nil {
		return
	}
	out<-f
}

func Logf(msg string, args ...interface{}) (int, error) {
	msg = fmt.Sprintf("[%v] [%v] %s", os.Getpid(), path.Base(os.Args[0]), msg)
	return fmt.Printf(msg, args...)
}

func CmdListen(cmd []string, out chan *os.File) error {
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
		f, err := fconn.File()
		if err != nil {
			conn.Close()
			continue
		}
		out<-f
	}
	return nil
}

func CmdConnect(cmd []string, out chan *os.File) {
	addr, err := url.Parse(cmd[1])
	if err != nil {
		return
	}
	conn, err := net.Dial(addr.Scheme, addr.Host)
	if err != nil {
		return
	}
	fconn, ok := conn.(interface { File() (*os.File, error) })
	if !ok {
		conn.Close()
		return
	}
	f, err := fconn.File()
	if err != nil {
		conn.Close()
		return
	}
	out<-f
}

func client(sockpath, command string) error {
	clientConn, err := dial(sockpath)
	if err != nil {
		return err
	}
	c, err := chord.NewClient(clientConn)
	endpoint, err := c.Open(command)
	if err != nil {
		return err
	}
	name, conn, err := endpoint.Receive()
	if err != nil {
		return err
	}
	fmt.Printf("--> %s\n", name)
	conn.Close()
	return nil
}


func dial(filename string) (*net.UnixConn, error) {
	addr, err := net.ResolveUnixAddr("unix", filename)
	if err != nil {
		return nil, fmt.Errorf("resolveaddr: %s", err)
	}
	return net.DialUnix("unix", nil, addr)
}

func listen(filename string) (*net.UnixListener, error) {
	addr, err := net.ResolveUnixAddr("unix", filename)
	if err != nil {
		return nil, fmt.Errorf("resolveaddr: %s", err)
	}
	return net.ListenUnix("unix", addr)
}
