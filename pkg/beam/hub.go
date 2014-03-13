package beam

import (
	"net"
	"strings"
	"os"
	"sync"
	"fmt"
)

func Hub() (*net.UnixConn, error) {
	inside, outside, err := usocketpair()
	if err != nil {
		return nil, err
	}
	go func() {
		defer inside.Close()
		routes := make(map[string]route)
		defer func() {
			fmt.Printf("[hub] closing all routes\n")
			for _, r := range routes {
				r.close()
			}
		}()
		for {
			fmt.Printf("[hub] waiting for new message\n")
			data, f, err := Receive(inside)
			if err != nil {
				return
			}
			fmt.Printf("[hub] received new message: '%s'\n", data)
			words := strings.Split(string(data), " ")
			var (
				r	route
				key	string
				index	int // 0 for payload, 1 for backend
			)
			if cmd := words[0]; cmd == "expose" {
				if len(words) < 2 {
					fmt.Printf("[hub] wrong use of expose. closing\n")
					// Usage: expose NAME
					// FIXME: return error via job/rpc protocol
					f.Close()
					continue
				}
				fmt.Printf("[hub] we have a backend\n")
				key = words[1]
				index = 1
			} else {
				fmt.Printf("[hub] we have a client connection\n")
				key = words[0]
				index = 0
			}
			r, exists := routes[key]
			if !exists {
				fmt.Printf("[hub] creating route for %s\n", key)
				routes[key] = newRoute()
				r = routes[key]
				go r.process()
			}
			// FIXME: if some backends are slow/overloaded, this will block
			// across all keys. Consider implementing a per-key backlog
			// which refuses messages if the target key is overloaded.
			fmt.Printf("[hub] sending message to route (on %v)\n", r[index])
			r[index] <- &msg{data: data, f: f}
			fmt.Printf("[hub] message passed to route\n")
		}
	}()
	return outside, nil
}


type route [2]chan *msg

func newRoute() route {
	return [2]chan *msg{
		make(chan *msg),
		make(chan *msg),
	}
}

func (r route) close() {
	close(r[0])
	close(r[1])
}

func (r route) process() {
	newConns := r[0]
	newBackends := r[1]
	conns := make(chan *msg)
	retryConns := make(chan *msg)
	backends := make(chan *msg)
	reuseBackends := make(chan *msg)
	var tasks sync.WaitGroup
	tasks.Add(3)
	// Merge newConns and retryConns into conns.
	go func() {
		defer tasks.Done()
		defer close(conns)
		fmt.Printf("[route] listening for new client connections (%v and %v)\n", newConns, retryConns)
		for {
			select {
				case conn, ok := <-retryConns: {
					if !ok {
						return
					}
					conns<-conn
				}
				case conn, ok := <-newConns: {
					fmt.Printf("[route] new connection!\n")
					if !ok {
						return
					}
					conns<-conn
				}
			}
		}
	}()
	// Merge newBackends and reuseBackends into backends.
	// No priority: mix all of them randomly.
	go func() {
		defer tasks.Done()
		defer close(backends)
		fmt.Printf("[route] listening for new backends (%v and %v)\n", newBackends, reuseBackends)
		for {
			select {
				case backend, ok := <-reuseBackends: {
					if !ok {
						return
					}
					backends<-backend
				}
				case backend, ok := <-newBackends: {
					fmt.Printf("[route] new backend!\n")
					if !ok {
						return
					}
					backends<-backend
				}
			}
		}
	}()
	// Get the next conn , then get the next backend, then send conn to backend.
	// If send is successful, send backend to reusebackends.
	// If send is not successful, send conn to retrycons.
	go func() {
		defer tasks.Done()
		defer close(retryConns)
		defer close(reuseBackends)
		for {
			conn, ok := <-conns
			if !ok {
				return
			}
			backendMsg, ok := <-backends
			if !ok {
				return
			}
			if backendMsg.f == nil {
				retryConns<-conn
				continue
			}
			backend, err := FdConn(int(backendMsg.f.Fd()))
			if err != nil {
				retryConns<-conn
				continue
			}
			if err := Send(backend, conn.data, conn.f); err != nil {
				retryConns<-conn
				continue
			}
			reuseBackends<-backendMsg
		}
	}()
	tasks.Wait()
}

type msg struct {
	data []byte
	f *os.File
}

func usocketpair() (*net.UnixConn, *net.UnixConn, error) {
	a, b, err := SocketPair()
	if err != nil {
		return nil, nil, err
	}
	uA, err := FdConn(int(a.Fd()))
	if err != nil {
		a.Close()
		b.Close()
		return nil, nil, err
	}
	uB, err := FdConn(int(b.Fd()))
	if err != nil {
		a.Close()
		b.Close()
		return nil, nil, err
	}
	return uA, uB, nil
}
