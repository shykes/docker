package bridge

import (
	"net"
	"os/exec"
	"strconv"
	"sync"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/docker/daemon/networkdriver/ipallocator"
	"github.com/docker/docker/daemon/networkdriver/portallocator"
	"github.com/docker/docker/daemon/networkdriver/portmapper"
	"github.com/docker/docker/engine"
	"github.com/docker/docker/nat"
	"github.com/docker/docker/network"
	"github.com/docker/docker/pkg/iptables"
)

const (
	DefaultNetworkBridge     = "docker0"
	MaxAllocatedPortAttempts = 10
)

// Network interface represents the networking stack of a container
type networkInterface struct {
	IP           net.IP
	PortMappings []net.Addr // there are mappings to the host interfaces
}

type ifaces struct {
	c map[string]*networkInterface
	sync.Mutex
}

func (i *ifaces) Set(key string, n *networkInterface) {
	i.Lock()
	i.c[key] = n
	i.Unlock()
}

func (i *ifaces) Get(key string) *networkInterface {
	i.Lock()
	res := i.c[key]
	i.Unlock()
	return res
}

var (
	addrs = []string{
		// Here we don't follow the convention of using the 1st IP of the range for the gateway.
		// This is to use the same gateway IPs as the /24 ranges, which predate the /16 ranges.
		// In theory this shouldn't matter - in practice there's bound to be a few scripts relying
		// on the internal addressing or other stupid things like that.
		// They shouldn't, but hey, let's not break them unless we really have to.
		"172.17.42.1/16", // Don't use 172.16.0.0/16, it conflicts with EC2 DNS 172.16.0.23
		"10.0.42.1/16",   // Don't even try using the entire /8, that's too intrusive
		"10.1.42.1/16",
		"10.42.42.1/16",
		"172.16.42.1/24",
		"172.16.43.1/24",
		"172.16.44.1/24",
		"10.0.42.1/24",
		"10.0.43.1/24",
		"192.168.42.1/24",
		"192.168.43.1/24",
		"192.168.44.1/24",
	}

	bridgeIface   string
	bridgeNetwork *net.IPNet

	defaultBindingIP  = net.ParseIP("0.0.0.0")
	currentInterfaces = ifaces{c: make(map[string]*networkInterface)}
)

func InitDriver(job *engine.Job) engine.Status {
	iface := job.Getenv("BridgeIface")
	bridgeIp, bridgeNet, err := net.ParseCIDR(job.Getenv("BridgeNet"))
	if err != nil {
		return job.Error(err)
	}
	bridgeNet.IP = bridgeIp
	if defaultIP := job.Getenv("DefaultBindingIP"); defaultIP != "" {
		defaultBindingIP = net.ParseIP(defaultIP)
	}
	bridgeIface = iface
	bridgeNetwork = bridgeNet

	// https://github.com/docker/docker/issues/2768
	job.Eng.Hack_SetGlobalVar("httpapi.bridgeIP", bridgeNetwork.IP)

	vxlanPeer := job.Getenv("VXLANPeer")

	if vxlanPeer != "" {
		if err := exec.Command("/sbin/ip", "link", "add", "vxlan0", "type", "vxlan", "id", "42", "remote", vxlanPeer).Run(); err != nil {
			return job.Error(err)
		}

		if err := exec.Command("/sbin/ip", "link", "set", "vxlan0", "up").Run(); err != nil {
			return job.Error(err)
		}

		if err := exec.Command("brctl", "addif", iface, "vxlan0").Run(); err != nil {
			return job.Error(err)
		}
	}

	for name, f := range map[string]engine.Handler{
		"allocate_interface": Allocate,
		"release_interface":  Release,
		"allocate_port":      AllocatePort,
		"link":               LinkContainers,
	} {
		if err := job.Eng.Register(name, f); err != nil {
			return job.Error(err)
		}
	}
	return engine.StatusOK
}

// Allocate a network interface
func Allocate(job *engine.Job) engine.Status {
	var (
		ip          net.IP
		mac         net.HardwareAddr
		err         error
		id          = job.Args[0]
		requestedIP = net.ParseIP(job.Getenv("RequestedIP"))
	)

	if requestedIP != nil {
		ip, err = ipallocator.RequestIP(bridgeNetwork, requestedIP)
	} else {
		ip, err = ipallocator.RequestIP(bridgeNetwork, nil)
	}
	if err != nil {
		return job.Error(err)
	}

	// If no explicit mac address was given, generate a random one.
	if mac, err = net.ParseMAC(job.Getenv("RequestedMac")); err != nil {
		mac = network.GenerateMacAddr(ip)
	}

	out := engine.Env{}
	out.Set("IP", ip.String())
	out.Set("Mask", bridgeNetwork.Mask.String())
	out.Set("Gateway", bridgeNetwork.IP.String())
	out.Set("MacAddress", mac.String())
	out.Set("Bridge", bridgeIface)

	size, _ := bridgeNetwork.Mask.Size()
	out.SetInt("IPPrefixLen", size)

	currentInterfaces.Set(id, &networkInterface{
		IP: ip,
	})

	out.WriteTo(job.Stdout)

	return engine.StatusOK
}

// release an interface for a select ip
func Release(job *engine.Job) engine.Status {
	var (
		id                 = job.Args[0]
		containerInterface = currentInterfaces.Get(id)
	)

	if containerInterface == nil {
		return job.Errorf("No network information to release for %s", id)
	}

	for _, nat := range containerInterface.PortMappings {
		if err := portmapper.Unmap(nat); err != nil {
			log.Infof("Unable to unmap port %s: %s", nat, err)
		}
	}

	if err := ipallocator.ReleaseIP(bridgeNetwork, containerInterface.IP); err != nil {
		log.Infof("Unable to release ip %s", err)
	}
	return engine.StatusOK
}

// Allocate an external port and map it to the interface
func AllocatePort(job *engine.Job) engine.Status {
	var (
		err error

		ip            = defaultBindingIP
		id            = job.Args[0]
		hostIP        = job.Getenv("HostIP")
		hostPort      = job.GetenvInt("HostPort")
		containerPort = job.GetenvInt("ContainerPort")
		proto         = job.Getenv("Proto")
		network       = currentInterfaces.Get(id)
	)

	if hostIP != "" {
		ip = net.ParseIP(hostIP)
		if ip == nil {
			return job.Errorf("Bad parameter: invalid host ip %s", hostIP)
		}
	}

	// host ip, proto, and host port
	var container net.Addr
	switch proto {
	case "tcp":
		container = &net.TCPAddr{IP: network.IP, Port: containerPort}
	case "udp":
		container = &net.UDPAddr{IP: network.IP, Port: containerPort}
	default:
		return job.Errorf("unsupported address type %s", proto)
	}

	//
	// Try up to 10 times to get a port that's not already allocated.
	//
	// In the event of failure to bind, return the error that portmapper.Map
	// yields.
	//

	var host net.Addr
	for i := 0; i < MaxAllocatedPortAttempts; i++ {
		if host, err = portmapper.Map(container, ip, hostPort); err == nil {
			break
		}

		if allocerr, ok := err.(portallocator.ErrPortAlreadyAllocated); ok {
			// There is no point in immediately retrying to map an explicitly
			// chosen port.
			if hostPort != 0 {
				job.Logf("Failed to bind %s for container address %s: %s", allocerr.IPPort(), container.String(), allocerr.Error())
				break
			}

			// Automatically chosen 'free' port failed to bind: move on the next.
			job.Logf("Failed to bind %s for container address %s. Trying another port.", allocerr.IPPort(), container.String())
		} else {
			// some other error during mapping
			job.Logf("Received an unexpected error during port allocation: %s", err.Error())
			break
		}
	}

	if err != nil {
		return job.Error(err)
	}

	network.PortMappings = append(network.PortMappings, host)

	out := engine.Env{}
	switch netAddr := host.(type) {
	case *net.TCPAddr:
		out.Set("HostIP", netAddr.IP.String())
		out.SetInt("HostPort", netAddr.Port)
	case *net.UDPAddr:
		out.Set("HostIP", netAddr.IP.String())
		out.SetInt("HostPort", netAddr.Port)
	}
	if _, err := out.WriteTo(job.Stdout); err != nil {
		return job.Error(err)
	}

	return engine.StatusOK
}

func LinkContainers(job *engine.Job) engine.Status {
	var (
		action       = job.Args[0]
		childIP      = job.Getenv("ChildIP")
		parentIP     = job.Getenv("ParentIP")
		ignoreErrors = job.GetenvBool("IgnoreErrors")
		ports        = job.GetenvList("Ports")
	)
	for _, value := range ports {
		port := nat.Port(value)
		if output, err := iptables.Raw(action, "FORWARD",
			"-i", bridgeIface, "-o", bridgeIface,
			"-p", port.Proto(),
			"-s", parentIP,
			"--dport", strconv.Itoa(port.Int()),
			"-d", childIP,
			"-j", "ACCEPT"); !ignoreErrors && err != nil {
			return job.Error(err)
		} else if len(output) != 0 {
			return job.Errorf("Error toggle iptables forward: %s", output)
		}

		if output, err := iptables.Raw(action, "FORWARD",
			"-i", bridgeIface, "-o", bridgeIface,
			"-p", port.Proto(),
			"-s", childIP,
			"--sport", strconv.Itoa(port.Int()),
			"-d", parentIP,
			"-j", "ACCEPT"); !ignoreErrors && err != nil {
			return job.Error(err)
		} else if len(output) != 0 {
			return job.Errorf("Error toggle iptables forward: %s", output)
		}
	}
	return engine.StatusOK
}
