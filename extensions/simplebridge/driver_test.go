package simplebridge

import (
	"io/ioutil"
	"testing"

	"github.com/docker/docker/state"

	"github.com/vishvananda/netlink"
)

/*
NOTE:

The interface allocator tries to get a FREE interface aggressively. This is
uncontrollable at this point. What this means, however, if the `test` network
is created and `test0` already exists, `test1` will be created by the driver.
The problem with this is mostly predictability within the tests.

So, we kinda sorta try to keep this stable by ensuring any `test0` device is
removed before the next set of tests run. See `createNetwork` and the
post-remove assertions.

The way forward is an "overwrite" or "fail on error" flag which controls this
behavior.
*/

func createNetwork(t *testing.T) *BridgeDriver {
	if link, err := netlink.LinkByName("test0"); err == nil {
		netlink.LinkDel(link)
	}

	driver := &BridgeDriver{}

	dir, err := ioutil.TempDir("", "simplebridge")
	if err != nil {
		t.Fatal(err)
	}

	extensionState, err := state.GitStateFromFolder(dir, "drivertest")
	if err != nil {
		t.Fatal(err)
	}

	if err := driver.Restore(extensionState); err != nil {
		t.Fatal(err)
	}

	driver.schema = NewSchema(driver.state)

	if err := driver.AddNetwork("test", []string{}); err != nil {
		t.Fatal(err)
	}

	return driver
}

func TestNetwork(t *testing.T) {
	driver := createNetwork(t)

	if _, err := netlink.LinkByName("test0"); err != nil {
		t.Fatal(err)
	}

	if _, err := driver.GetNetwork("test"); err != nil {
		t.Fatal("Fetching network 'test' did not succeed")
	}

	if link, _ := netlink.LinkByName("test0"); link == nil {
		t.Fatalf("Could not find %q link", "test")
	}

	if err := driver.RemoveNetwork("test"); err != nil {
		t.Fatal(err)
	}

	if link, _ := netlink.LinkByName("test0"); link != nil {
		t.Fatalf("link %q still exists after RemoveNetwork", "test")
	}
}

func TestEndpoint(t *testing.T) {
	driver := createNetwork(t)

	if link, err := netlink.LinkByName("ept"); err == nil {
		netlink.LinkDel(link)
	}

	if _, err := driver.Link("test", "ept", nil, true); err != nil {
		t.Fatal(err)
	}

	if _, err := netlink.LinkByName("ept"); err != nil {
		t.Fatal(err)
	}

	if _, err := netlink.LinkByName("ept-int"); err != nil {
		t.Fatal(err)
	}

	if err := driver.Unlink("test", "ept", nil); err != nil {
		t.Fatal(err)
	}

	if err := driver.RemoveNetwork("test"); err != nil {
		t.Fatal(err)
	}
}
