// +build daemon

package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"strings"

	"code.google.com/p/go.crypto/ssh"
	"github.com/docker/docker/engine"
	"github.com/docker/docker/daemon"
	"github.com/docker/docker/runconfig"
	"github.com/docker/docker/pkg/sysinfo"
)

func serveSSH(eng *engine.Engine, daemon *daemon.Daemon) error {
	// An SSH server is represented by a ServerConfig, which holds
	// certificate details and handles authentication of ServerConns.
	config := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			// Should use constant-time compare (or better, salt+hash) in
			// a production setting.
			if c.User() == "docker" && string(pass) == "docker" {
				return nil, nil
			}
			return nil, fmt.Errorf("password rejected for %q", c.User())
		},
	}

	// FIXME integrate with libtrust
	privateBytes, err := ioutil.ReadFile("/var/lib/docker/ssh_host_key")
	if err != nil {
		return fmt.Errorf("Failed to load private key")
	}

	private, err := ssh.ParsePrivateKey(privateBytes)
	if err != nil {
		return fmt.Errorf("Failed to parse private key")
	}

	config.AddHostKey(private)

	// Once a ServerConfig has been configured, connections can be
	// accepted.
	listener, err := net.Listen("tcp", "0.0.0.0:2377")
	if err != nil {
		return fmt.Errorf("failed to listen for connection")
	}
	for {
		nConn, err := listener.Accept()
		if err != nil {
			return fmt.Errorf("failed to accept incoming connection")
		}

		// Handle new connection
		go func(conn net.Conn) {
			err := serveSSHConn(eng, daemon, conn, config)
			if err != nil {
				log.Printf("ssh error: %v\n", err)
			}
		}(nConn)
	}
	return nil
}

func serveSSHConn(eng *engine.Engine, daemon *daemon.Daemon, nConn net.Conn, config *ssh.ServerConfig) error {
	// Before use, a handshake must be performed on the incoming
	// net.Conn.
	_, chans, reqs, err := ssh.NewServerConn(nConn, config)
	if err != nil {
		return fmt.Errorf("failed to handshake")
	}
	// The incoming Request channel must be serviced.
	go ssh.DiscardRequests(reqs)

	// Service the incoming Channel channel.
	for newChannel := range chans {
		// Channels have a type, depending on the application level
		// protocol intended. In the case of a shell, the type is
		// "session" and ServerShell may be used to present a simple
		// terminal interface.
		// fmt.Printf("--> NEWCHAN '%s' '%s'\n", newChannel.ChannelType(), newChannel.ExtraData())
		if newChannel.ChannelType() != "session" {
			newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}
		channel, requests, err := newChannel.Accept()
		if err != nil {
			return fmt.Errorf("could not accept channel.")
		}

		// Handle new channel
		go func(channel ssh.Channel, requests <-chan *ssh.Request) {
			for req := range requests {
				// fmt.Printf("--> REQ %#v\n", req)
				switch req.Type {
					case "exec": {
						args := strings.Split(string(req.Payload[4:]), " ")
						fmt.Printf("---> exec |%v|\n", args)
						if err := handleExec(eng, daemon, channel, args); err != nil {
							req.Reply(false, nil)
						} else {
							req.Reply(true, nil)
						}
						channel.Close()
					}
					default: {
						req.Reply(false, nil)
						continue
					}
				}
			}
		}(channel, requests)
	}
	return nil
}

func handleExec(eng *engine.Engine, daemon *daemon.Daemon, channel ssh.Channel, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("exec: no arguments")
	}
	switch args[0] {
		case "containers": {
			job := eng.Job("containers")
			job.Env().SetBool("all", true)
			job.Stdout.Add(channel)
			job.Stderr.Add(channel.Stderr())
			job.Run()
		}
		case "run": {
			config, hostconfig, _, err := runconfig.Parse(args[1:], new(sysinfo.SysInfo))
			if err != nil {
				return err
			}
			c, warnings, err := daemon.Create(config, "")
			if err != nil {
				return err
			}
			for _, w := range warnings {
				fmt.Fprintf(channel.Stderr(), "[WARNING] %s\n", w)
			}
			c.SetHostConfig(hostconfig)
			errors := daemon.Attach(c, channel, channel, channel, channel.Stderr())
			if err := c.Start(); err != nil {
				return err
			}
			for err := range errors {
				if err != nil {
					return err
				}
			}
			return nil
		}
		case "attach": {
			job := eng.Job("attach", args[1:]...)
			job.Env().SetBool("stdout", true)
			job.Env().SetBool("stderr", true)
			job.Env().SetBool("stdin", true)
			job.Env().SetBool("stream", true)
			job.Stdout.Add(channel)
			job.Stderr.Add(channel.Stderr())
			job.Stdin.Add(channel)
			job.Run()
		}
		default: {
			fmt.Fprintf(channel.Stderr(), "no such command: %s", args[0])
		}
	}
	return nil
}
