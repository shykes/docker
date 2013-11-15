## Docker 0.7.0 RC5

You can download the binary for the 0.7.0 RC5 release [here](https://github.com/crosbymichael/docker/tree/0.7.0-rc5).  This is 
a static binary with all dependencies included.  *This RC should not be used over the top of an existing docker install!*

You will still need to ensure that `tar` and `lxc` are installed on your system before running docker.  

```bash
# if you are running fedora 

sudo yum install lxc
sudo yum install tar


# download to somewhere in your PATH
wget -O docker test.docker.io/builds/Linux/x86_64/docker-0.7.0-rc5
chmod +x docker
sudo docker -d -D 

# daemon should now be running
```

You can run the docker daemon from the binary via `docker -d -D`.  I would suggest using the `-D` option to receive debug output from the daemon, this is a RC after all, incase 
you encounter an issue and need to provide a very helpful report.

### Storage driver architecture 
The big change with 0.7.0 is that docker will support multiple drivers for storing and mounting layered/CoW filesystems.  AUFS and devicemapper are both implemented in this release.
If you system does not support AUFS docker will use devicemapper for it's CoW filesystem.  You can manually set your driver of choice in this RC with the env var
`DOCKER_DRIVER=aufs|devicemapper`.

Although the driver API can still change between this RC and the final release, currently, if you would like to write a driver for 
docker you have to implement the following interface:

```golang
type Driver interface {
	Create(id, parent string) error // Create a new dir with id and a parent id
	Get(id string) (dir string, err error) // Return the mount point for the given id

	Remove(id string) error // Remove the dir for the given id
	Size(id string) (bytes int64, err error) // Return the size of the dir for the given id
	Cleanup() error // Docker is shutting down and you should cleanup any existing mount points to any driver specific tasks
}
```

The new architecture allows us to support more drivers without affecting the core docker codebase. 

### Known Issues 
This RC is very close to be the final 0.7.0 release but it does contain a few know issues.

* Diff size is not correctly reported with devicemapper
* Volumes with existing content are not correctly setup
* Existing docker installs are not supported - You must use a clean install of /var/lib/docker for this RC
* You Cannot toggle between drivers with the same /var/lib/docker dir, you must remove /var/lib/docker first before switching drivers


