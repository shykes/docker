package simplebridge

import (
	"io/ioutil"
	"net"
	"os"
	"strings"
	"testing"
)

func createTestAllocate(ok bool) allocateFunc {
	return func(dstIP net.IP) bool {
		return ok
	}
}

func createTestRefresh(t *testing.T, arpfile string) refreshFunc {
	return func(_if *net.Interface) (map[string]struct{}, error) {
		f, err := os.Open(arpfile)
		if err != nil {
			return nil, err
		}

		defer f.Close()

		content, err := ioutil.ReadAll(f)
		if err != nil {
			return nil, err
		}

		ipMap := map[string]struct{}{}

		for _, ip := range strings.Split(string(content), "\n") {
			ipMap[strings.TrimSpace(ip)] = struct{}{}
		}

		return ipMap, nil
	}
}

func TestAllocateEmpty(t *testing.T) {
	tr := createTestRefresh(t, os.DevNull)
	ta := createTestAllocate(true)
	bridgeIP, ipNet, err := net.ParseCIDR("172.16.0.1/16")

	if err != nil {
		t.Fatal(err)
	}

	createNetwork(t) // XXX from driver_test.go

	ip := NewIPAllocator("test", ipNet, tr, ta)
	allocated, err := ip.Allocate()
	if err != nil {
		t.Fatal(err)
	}

	if !allocated.Equal(bridgeIP) {
		t.Fatalf("Allocated ip was %q, expected was %q", allocated.String(), bridgeIP.String())
	}
}

func TestAllocateBasic(t *testing.T) {
	tr := createTestRefresh(t, "testdata/ipallocator/arptable1")
	ta := createTestAllocate(true)
	bridgeIP, ipNet, err := net.ParseCIDR("172.16.0.1/16")

	if err != nil {
		t.Fatal(err)
	}

	createNetwork(t) // XXX from driver_test.go

	ip := NewIPAllocator("test", ipNet, tr, ta)
	allocated, err := ip.Allocate()
	if err != nil {
		t.Fatal(err)
	}

	if !allocated.Equal(net.ParseIP("172.16.0.3")) {
		t.Fatalf("Allocated ip was %q, expected was %q", allocated.String(), bridgeIP.String())
	}
}

func TestAllocateCycle(t *testing.T) {
	tr := createTestRefresh(t, "testdata/ipallocator/arptable2")
	ta := createTestAllocate(true)
	_, ipNet, err := net.ParseCIDR("172.16.0.1/29")

	if err != nil {
		t.Fatal(err)
	}

	createNetwork(t) // XXX from driver_test.go

	ip := NewIPAllocator("test", ipNet, tr, ta)
	if allocated, err := ip.Allocate(); err == nil {
		t.Fatalf("Did not error; should have cycled trying to allocate on %q: got %q", ipNet.String(), allocated.String())
	}
}
