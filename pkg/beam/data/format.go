package data

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

func (m Msg) GetInt(k string) (int64, error) {
	s := strings.Trim(m.Get(k), " \t")
	return strconv.ParseInt(s, 10, 64)
}

func (m Msg) SetInt(k string, v int64) {
	m.Set(k, fmt.Sprintf("%d", v))
}

func (m Msg) GetUint(k string) (uint64, error) {
	s := strings.Trim(m.Get(k), " \t")
	return strconv.ParseUint(s, 10, 64)
}

func (m Msg) GetMsg(k string) (value Msg, err error) {
	value = make(Msg)
	if !m.Exists(k) {
		return
	}
	_, err = value.ReadFrom(strings.NewReader(m.Get(k)))
	return
}

func (m Msg) ToHTTPHeader() http.Header {
	h := make(http.Header)
	// Sort the keys to guarantee a deterministic output.
	// This is necessary because multiple message keys might be folded
	// into a single header key (because we are dropping the case sensitivity).
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		for _, v := range m[k] {
			h.Add(k, v)
		}
	}
	return h
}
