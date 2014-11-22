# VXLAN setup documentation

## Configuration

Two machines, 10.0.1.103 and 10.0.1.104. 

First machine, 10.0.1.103:

```bash
ip link add vxlan0 type vxlan id 42 remote 10.0.1.104 dev eth0
```

Second Machine, 10.0.1.104:
```bash
ip link add vxlan0 type vxlan id 42 remote 10.0.1.104 dev eth0
```

Both machines:
```bash
ip link set vxlan0 up
docker -d -D &
brctl addif docker0 vxlan0
```

Then on the first machine, run a container (busybox is fine) with a shell. On
the second one, run one, then terminate it and run another. This is because of
the way the ip allocator works in combination with the subnets, it will cause
arp issues.

Ping container A (likely 172.17.0.2) from container B (likely 172.17.0.3). It should work.

Now on each container:

```bash
apt-get update && apt-get install iperf -y
```

Then on one (doesn't matter):

```bash
iperf -s
```

And on the other:
```bash
iperf -c <server ip>
```

The box might panic at this point. We believe this is related to kvm or smartos
which was in my initial setup. Confirmed working just fine on vmware.
