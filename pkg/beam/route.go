package beam

type Route struct {
	filters []func(*Stream) bool
	fn      func(*Stream)
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

func (r *Route) HandleFunc(fn func(*Stream)) *Route {
	r.fn = fn
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

func (r *Route) Handle(st *Stream) {
	if r.fn == nil {
		return
	}
	r.fn(st)
}
