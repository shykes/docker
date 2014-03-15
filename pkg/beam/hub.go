package beam

import (
	"net"
	"strings"
	"os"
	"sync"
	"fmt"
)

func Hub() (*net.UnixConn, error) {
	inside, outside, err := USocketPair()
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
			clientMsg, ok := <-conns
			if !ok {
				return
			}
			fmt.Printf("[route] next client connection to route: %v\n", clientMsg)
			if clientMsg.f == nil {
				continue
			}
			fmt.Printf("[route] next client connection to route: %v\n", clientMsg)
			client, err := FdConn(int(clientMsg.f.Fd()))
			if err != nil {
				continue
			}
			backendMsg, ok := <-backends
			if !ok {
				return
			}
			if backendMsg.f == nil {
				retryConns<-clientMsg
				continue
			}
			backend, err := FdConn(int(backendMsg.f.Fd()))
			if err != nil {
				retryConns<-clientMsg
				continue
			}
			fmt.Printf("[route] backend and client ready to join. creating socketpair\n")
			a, b, err := SocketPair()
			if err != nil {
				panic(fmt.Sprintf("can't create socket pair: %v", err))
			}
			fmt.Printf("[route] sending to backend fd=%v\n", a.Fd())
			if err := Send(backend, clientMsg.data, a); err != nil {
				a.Close()
				b.Close()
				retryConns<-clientMsg
				continue
			}
			fmt.Printf("[route] sending to client fd=%v\n", b.Fd())
			if err := Send(client, backendMsg.data, b); err != nil {
				a.Close()
				b.Close()
				reuseBackends<-backendMsg
				continue
			}
			// At this point, incredibly, everything worked.
			reuseBackends<-backendMsg
		}
	}()
	tasks.Wait()
}

type msg struct {
	data []byte
	f *os.File
}

