// +build linux
package network

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"syscall"

	"github.com/docker/docker/pkg/netlink"
	"github.com/docker/docker/pkg/reexec"
	"github.com/docker/libcontainer/system"
)

const DefaultInterfaceName = "eth0"

type setupError struct {
	Message string `json:"message,omitempty"`
}

func (i setupError) Error() string {
	return i.Message
}

type Interface struct {
	// Iface is interface name
	Iface string
	// MacAddress contains the MAC address to set on the network interface
	MacAddress string `json:"mac_address,omitempty"`
	// Address contains the IPv4 and mask to set on the network interface
	Address string `json:"address,omitempty"`
	// IPv6Address contains the IPv6 and mask to set on the network interface
	IPv6Address string `json:"ipv6_address,omitempty"`
	// Gateway sets the gateway address that is used as the default for the interface
	Gateway string `json:"gateway,omitempty"`
	// IPv6Gateway sets the ipv6 gateway address that is used as the default for the interface
	IPv6Gateway string `json:"ipv6_gateway,omitempty"`
	// Mtu sets the mtu value for the interface and will be mirrored on both the host and
	// container's interfaces if a pair is created, specifically in the case of type veth
	// Note: This does not apply to loopback interfaces.
	Mtu    int `json:"mtu,omitempty"`
	NsPath string
}

const nsBin = "setupnetns"

func init() {
	reexec.Register(nsBin, setupInterface)
}

func newInitPipe() (parent *os.File, child *os.File, err error) {
	fds, err := syscall.Socketpair(syscall.AF_LOCAL, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, nil, err
	}
	return os.NewFile(uintptr(fds[1]), "parent"), os.NewFile(uintptr(fds[0]), "child"), nil
}

func setupInterface() {
	f := os.NewFile(3, "child")
	var (
		i   Interface
		err error
	)
	defer func() {
		if err != nil {
			ioutil.ReadAll(f)
			if err := json.NewEncoder(f).Encode(setupError{Message: err.Error()}); err != nil {
				panic(err)
			}
		}
		f.Close()
	}()
	if err = json.NewDecoder(f).Decode(&i); err != nil {
		return
	}
	if err = i.setupInNs(); err != nil {
		return
	}
}

func (i *Interface) Setup(nsPath string) error {
	parent, child, err := newInitPipe()
	if err != nil {
		return err
	}
	defer parent.Close()
	cmd := &exec.Cmd{
		Path: reexec.Self(),
		Args: []string{nsBin, nsPath},
	}
	cmd.ExtraFiles = []*os.File{child}
	if err := cmd.Start(); err != nil {
		child.Close()
		return err
	}
	child.Close()
	if err := json.NewEncoder(parent).Encode(i); err != nil {
		return err
	}
	if err := syscall.Shutdown(int(parent.Fd()), syscall.SHUT_WR); err != nil {
		return err
	}
	var serr *setupError
	if err := json.NewDecoder(parent).Decode(&serr); err != nil && err != io.EOF {
		return err
	}
	if serr != nil {
		return serr
	}
	return nil
}

func (i *Interface) setupInNs() error {
	nsPath := os.Args[1]
	if err := SetInterfaceInNamespacePath(i.Iface, nsPath); err != nil {
		return err
	}
	f, err := os.OpenFile(nsPath, os.O_RDONLY, 0)
	if err != nil {
		return fmt.Errorf("failed get network namespace fd: %v", err)
	}

	// switching to netns
	err = system.Setns(f.Fd(), syscall.CLONE_NEWNET)
	f.Close()
	if err != nil {
		return err
	}
	if err := InterfaceDown(i.Iface); err != nil {
		return fmt.Errorf("interface down %s %s", i.Iface, err)
	}
	if err := ChangeInterfaceName(i.Iface, DefaultInterfaceName); err != nil {
		return fmt.Errorf("change %s to %s %s", i.Iface, DefaultInterfaceName, err)
	}
	if i.MacAddress != "" {
		if err := SetInterfaceMac(DefaultInterfaceName, i.MacAddress); err != nil {
			return fmt.Errorf("set %s mac %s", DefaultInterfaceName, err)
		}
	}
	if err := SetInterfaceIp(DefaultInterfaceName, i.Address); err != nil {
		return fmt.Errorf("set %s ip %s", DefaultInterfaceName, err)
	}
	if i.IPv6Address != "" {
		if err := SetInterfaceIp(DefaultInterfaceName, i.IPv6Address); err != nil {
			return fmt.Errorf("set %s ipv6 %s", DefaultInterfaceName, err)
		}
	}

	if err := SetMtu(DefaultInterfaceName, i.Mtu); err != nil {
		return fmt.Errorf("set %s mtu to %d %s", DefaultInterfaceName, i.Mtu, err)
	}
	if err := InterfaceUp(DefaultInterfaceName); err != nil {
		return fmt.Errorf("%s up %s", DefaultInterfaceName, err)
	}
	if i.Gateway != "" {
		if err := SetDefaultGateway(i.Gateway, DefaultInterfaceName); err != nil {
			return fmt.Errorf("set gateway to %s on device %s failed with %s", i.Gateway, DefaultInterfaceName, err)
		}
	}
	if i.IPv6Gateway != "" {
		if err := SetDefaultGateway(i.IPv6Gateway, DefaultInterfaceName); err != nil {
			return fmt.Errorf("set gateway for ipv6 to %s on device %s failed with %s", i.IPv6Gateway, DefaultInterfaceName, err)
		}
	}
	return nil
}

func InterfaceUp(name string) error {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return err
	}
	return netlink.NetworkLinkUp(iface)
}

func InterfaceDown(name string) error {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return err
	}
	return netlink.NetworkLinkDown(iface)
}

func ChangeInterfaceName(old, newName string) error {
	iface, err := net.InterfaceByName(old)
	if err != nil {
		return err
	}
	return netlink.NetworkChangeName(iface, newName)
}

func CreateVethPair(name1, name2 string, txQueueLen int) error {
	return netlink.NetworkCreateVethPair(name1, name2, txQueueLen)
}

func SetInterfaceInNamespacePid(name string, nsPid int) error {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return err
	}
	return netlink.NetworkSetNsPid(iface, nsPid)
}

func SetInterfaceInNamespaceFd(name string, fd uintptr) error {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return err
	}
	return netlink.NetworkSetNsFd(iface, int(fd))
}

func SetInterfaceInNamespacePath(name string, path string) error {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	fd := f.Fd()
	return netlink.NetworkSetNsFd(iface, int(fd))
}

func SetInterfaceMaster(name, master string) error {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return err
	}
	masterIface, err := net.InterfaceByName(master)
	if err != nil {
		return err
	}
	return netlink.AddToBridge(iface, masterIface)
}

func SetDefaultGateway(ip, ifaceName string) error {
	return netlink.AddDefaultGw(ip, ifaceName)
}

func SetInterfaceMac(name string, macaddr string) error {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return err
	}
	return netlink.NetworkSetMacAddress(iface, macaddr)
}

func SetInterfaceIp(name string, rawIp string) error {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return err
	}
	ip, ipNet, err := net.ParseCIDR(rawIp)
	if err != nil {
		return err
	}
	return netlink.NetworkLinkAddIp(iface, ip, ipNet)
}

func SetMtu(name string, mtu int) error {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return err
	}
	return netlink.NetworkSetMTU(iface, mtu)
}

func SetHairpinMode(name string, enabled bool) error {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return err
	}
	return netlink.SetHairpinMode(iface, enabled)
}
