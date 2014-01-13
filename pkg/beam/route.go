package beam

import (
	"fmt"
	"io"
)

func NewRouter() *Router {
	return &Router{}
}

type Router struct {
	routes []*Route
}

func (r *Router) NewRoute() *Route {
	route := &Route{}
	r.routes = append(r.routes, route)
	return route
}

func (r *Router) Send(st *Stream) error {
	for i := range r.routes {
		// Last route added wins
		route := r.routes[len(r.routes)-i-1]
		if route.Match(st) {
			return route.Send(st)
		}
	}
	// If it didn't match, silently drop it
	return nil
}

func (r *Router) Close() error {
	return nil
}

func (r *Router) ReceiveFrom(src Receiver) error {
	for {
		st, err := src.Receive()
		if st != nil {
			r.Send(st)
		}
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}
	}
	return nil
}

type Route struct {
	filters []func(*Stream) bool
	handler	Sender
}

type Sender interface {
	Send(*Stream) error
}

type Receiver interface {
	Receive() (*Stream, error)
}

func (r *Route) Send(st *Stream) error {
	if r.handler == nil {
		return fmt.Errorf("route has no handler")
	}
	return r.handler.Send(st)
}

func (r *Route) Handler(h Sender) *Route {
	r.handler = h
	return r
}

func (r *Route) Parent(parentIds ...int) *Route {
	r.filters = append(r.filters, func(st *Stream) (match bool) {
		parent := st.Parent()
		if parent == nil {
			return len(parentIds) == 0
		}
		for _, parentId := range parentIds {
			if parent.Id() == parentId {
				return true
			}
		}
		return false
	})
	return r
}

func (r *Route) Name(name string) *Route {
	return r.Headers("name", name)
}

func (r *Route) Headers(pairs ...string) *Route {
	r.filters = append(r.filters, func(st *Stream) (match bool) {
		for i := 0; i < len(pairs); i += 2 {
			key := pairs[i]
			var value string
			if len(pairs) > i+1 {
				value = pairs[i+1]
			}
			if value == "" {
				if !st.Metadata.Exists(key) {
					return false
				}
				continue
			}
			if st.Metadata.Get(key) != value {
				return false
			}
		}
		return true
	})
	return r
}

func (r *Route) MatcherFunc(filter func(*Stream) bool) *Route {
	r.filters = append(r.filters, filter)
	return r
}

func (r *Route) Match(st *Stream) (match bool) {
	for _, filter := range r.filters {
		if filter(st) == false {
			return false
		}
	}
	return true
}

func (r *Route) HandleFunc(fn func(*Stream)) *Route {
	r.handler = HandleFunc(fn)
	return r
}

type HandleFunc func(*Stream)

func (h HandleFunc) Send(st *Stream) error {
	go h(st)
	return nil
}
