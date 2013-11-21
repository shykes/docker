How to test the RC
==============

The docker daemon will try to use `aufs`, and if it's not possible it'll try to use `devmapper`.
So if you already have a docker setup working with `aufs`, it'll work like it did before.

***Be careful, there is a small change in way we store images and containers, so once you used docker-0.7.0-rc7, you can't go back to the docker-0.6.x branch***

*You can still use the `-g` flag in the daemon to specify a different storage directory* 

    #kill the docker daemon
    pkill docker

    wget https://test.docker.io/builds/Linux/x86_64/docker-0.7.0-rc7
    chmod +x docker-0.7.0-rc7
    mv docker-0.7.0-rc7 <path to you existing docker daemon>

    #restart the daemon
    docker -d

Otherwise, if you are on `Fedora` do:

    #install lxc
    yum install-lxc
 
    wget https://test.docker.io/builds/Linux/x86_64/docker-0.7.0-rc7
    chmod +x docker-0.7.0-rc7
    mv docker-0.7.0-rc7 /usr/local/bin/

    #start the daemon
    docker -d

Forcing a backend
=============

You can use the `-graph-driver` flag on the daemon to choose the backend you want to use.

    -graph-driver=aufs
    -graph-driver=devmapper
    -graph-driver=dummy #simple copy, for test purpose only


Migration
=======

Currently we don't support migration, so if you have containers and images on let's say `aufs` and you start the daemon in `devmapper` You won't see your images/containers. This is expected.
You can still `docker import` / `docker export` them.
