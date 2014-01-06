package beam

import (
	"fmt"
	"sync"
)

type Server struct {
	session *Session
	routes  []*Route
}

func (srv *Server) Session() *Session {
	return srv.session
}

func NewServer(session *Session) *Server {
	return &Server{session: session}
}

func (srv *Server) NewRoute() *Route {
	route := &Route{}
	srv.routes = append(srv.routes, route)
	return route
}

func (srv *Server) Serve() error {
	var wg sync.WaitGroup
	defer wg.Wait()
	for {
		st, err := srv.session.Receive()
		if err != nil {
			return fmt.Errorf("receive: %s", err)
		}
		fmt.Printf("+++ %d %s\n", st.Id(), st.Metadata.ShortString())
		for i := range srv.routes {
			// Last route added wins
			route := srv.routes[len(srv.routes)-i-1]
			if route.Match(st) {
				wg.Add(1)
				go func() {
					route.Handle(st)
					st.Close()
					wg.Done()
				}()
				continue
			}
		}
		fmt.Printf("No matching route for inbound stream %d. Dropping\n", st.Id())
	}
}
