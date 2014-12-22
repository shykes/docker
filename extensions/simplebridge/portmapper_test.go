package simplebridge

import (
	"net"
	"testing"

	"github.com/docker/docker/pkg/iptables"
)

type testPortMap struct {
	// action is special because it's not actually used by production code. It's
	// used by the test suite to store additional data when testing forward rules.
	action  iptables.Action
	portmap PortMap
}

var testForwardMap = map[string]testPortMap{}

func init() {
	// XXX this variable is top-level and comes from portmapper.go
	portTableFormat = "testdata/portmapper/%s"
}

func testForward(chainName string, action iptables.Action, proto string, hostIP net.IP, hostPort uint, containerIP net.IP, containerPort uint) error {
	testForwardMap[chainName] = testPortMap{
		action: action,
		portmap: PortMap{
			proto:         proto,
			hostIP:        hostIP,
			hostPort:      hostPort,
			containerIP:   containerIP,
			containerPort: containerPort,
		},
	}

	return nil
}

func resetMap() {
	testForwardMap = map[string]testPortMap{}
}

func TestMap(t *testing.T) {
	resetMap()

	defaultChain := "DOCKER"
	MakeChain(defaultChain, "test")

	// the testdata has 111, 22, and 44210 open on 0/0 tcp and tcp6
	if err := NewPortMap(defaultChain, net.ParseIP("0.0.0.0"), "tcp", net.ParseIP("123.123.123.123"), 0, 1234, testForward).Map(); err != nil {
		t.Fatal(err)
	}

	fwd := testForwardMap[defaultChain]
	pm := fwd.portmap

	if fwd.action != iptables.Add ||
		pm.proto != "tcp" ||
		!pm.hostIP.Equal(net.ParseIP("0.0.0.0")) ||
		pm.hostPort != 1234 ||
		!pm.containerIP.Equal(net.ParseIP("123.123.123.123")) ||
		pm.containerPort != 0 {

		t.Fatal("Mapping for 1234 failed due to incorrect forward parameters")
	}

	if _, ok := hostPortMap[pm.hostPort]; !ok {
		t.Fatalf("hostPort %q was not mapped", pm.hostPort)
	}

	if err := NewPortMap(defaultChain, net.ParseIP("0.0.0.0"), "tcp", net.ParseIP("123.123.123.123"), 0, 22, testForward).Map(); err == nil {
		t.Fatal("Error was supposed to be returned mapping port 22, succeeded")
	}

	if err := NewPortMap(defaultChain, net.ParseIP("123.123.123.124"), "tcp", net.ParseIP("123.123.123.123"), 0, 22, testForward).Map(); err == nil {
		t.Fatal("Error was supposed to be returned mapping port 22, succeeded")
	}

	// port 25 listens only on 127.0.0.1
	if err := NewPortMap(defaultChain, net.ParseIP("0.0.0.0"), "tcp", net.ParseIP("123.123.123.123"), 0, 25, testForward).Map(); err == nil {
		t.Fatal("Error was supposed to be returned mapping port 25, succeeded")
	}
}

func TestUnmap(t *testing.T) {
	resetMap()

	defaultChain := "DOCKER"
	MakeChain(defaultChain, "test")

	if err := NewPortMap(defaultChain, net.ParseIP("0.0.0.0"), "tcp", net.ParseIP("123.123.123.123"), 0, 1234, testForward).Unmap(); err != nil {
		t.Fatal(err)
	}

	fwd := testForwardMap[defaultChain]
	pm := fwd.portmap

	if fwd.action != iptables.Delete ||
		pm.proto != "tcp" ||
		!pm.hostIP.Equal(net.ParseIP("0.0.0.0")) ||
		pm.hostPort != 1234 ||
		!pm.containerIP.Equal(net.ParseIP("123.123.123.123")) ||
		pm.containerPort != 0 {

		t.Fatal("Unmapping for 1234 failed due to incorrect forward parameters")
	}

	if _, ok := hostPortMap[pm.hostPort]; ok {
		t.Fatalf("Mapped port %q still exists after Unmap", pm.hostPort)
	}
}

func TestMapIPv6(t *testing.T) {
	resetMap()

	defaultChain := "DOCKER"
	MakeChain(defaultChain, "test")

	// the testdata has 111, 22, and 44210 open on 0/0 tcp and tcp6
	if err := NewPortMap(defaultChain, net.ParseIP("::"), "tcp", net.ParseIP("fe80::1"), 0, 1234, testForward).Map(); err != nil {
		t.Fatal(err)
	}

	fwd := testForwardMap[defaultChain]
	pm := fwd.portmap

	if fwd.action != iptables.Add ||
		pm.proto != "tcp" ||
		!pm.hostIP.Equal(net.ParseIP("::")) ||
		pm.hostPort != 1234 ||
		!pm.containerIP.Equal(net.ParseIP("fe80::1")) ||
		pm.containerPort != 0 {

		t.Fatal("Mapping for 1234 failed due to incorrect forward parameters")
	}

	if _, ok := hostPortMap[pm.hostPort]; !ok {
		t.Fatalf("hostPort %q was not mapped", pm.hostPort)
	}

	if err := NewPortMap(defaultChain, net.ParseIP("::"), "tcp", net.ParseIP("fe80::1"), 0, 22, testForward).Map(); err == nil {
		t.Fatal("Error was supposed to be returned mapping port 22, succeeded")
	}

	if err := NewPortMap(defaultChain, net.ParseIP("fe80::2"), "tcp", net.ParseIP("fe80::1"), 0, 22, testForward).Map(); err == nil {
		t.Fatal("Error was supposed to be returned mapping port 22, succeeded")
	}

	// port 25 listens only on 127.0.0.1
	if err := NewPortMap(defaultChain, net.ParseIP("::"), "tcp", net.ParseIP("fe80::1"), 0, 25, testForward).Map(); err == nil {
		t.Fatal("Error was supposed to be returned mapping port 25, succeeded")
	}
}

func TestUnmapIPv6(t *testing.T) {
	resetMap()

	defaultChain := "DOCKER"
	MakeChain(defaultChain, "test")

	if err := NewPortMap(defaultChain, net.ParseIP("::"), "tcp", net.ParseIP("fe80::1"), 0, 1234, testForward).Unmap(); err != nil {
		t.Fatal(err)
	}

	fwd := testForwardMap[defaultChain]
	pm := fwd.portmap

	if fwd.action != iptables.Delete ||
		pm.proto != "tcp" ||
		!pm.hostIP.Equal(net.ParseIP("::")) ||
		pm.hostPort != 1234 ||
		!pm.containerIP.Equal(net.ParseIP("fe80::1")) ||
		pm.containerPort != 0 {

		t.Fatal("Unmapping for 1234 failed due to incorrect forward parameters")
	}

	if _, ok := hostPortMap[pm.hostPort]; ok {
		t.Fatalf("Mapped port %q still exists after Unmap", pm.hostPort)
	}
}
