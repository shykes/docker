package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/dotcloud/docker/api"
	"github.com/dotcloud/docker/builtins"
	"github.com/dotcloud/docker/dockerversion"
	"github.com/dotcloud/docker/engine"
	flag "github.com/dotcloud/docker/pkg/mflag"
	"github.com/dotcloud/docker/pkg/opts"
	"github.com/dotcloud/docker/sysinit"
	"github.com/dotcloud/docker/utils"
)

func main() {
	if selfPath := utils.SelfPath(); selfPath == "/sbin/init" || selfPath == "/.dockerinit" {
		// Running in init mode
		sysinit.SysInit()
		return
	}

	var opts Opts
	opts.Parse()
	if opts.Version {
		showVersion()
		return
	}
	if opts.Debug {
		os.Setenv("DEBUG", "1")
	}

	if len(opts.Args) == 0 {
		flag.Usage()
		os.Exit(1)
	}
	eng, err := engine.New(".docker")
	if err != nil {
		log.Fatal(err)
	}
	// Register builtins
	builtins.Register(eng)
	// Load plugins
	for _, pluginCmd := range opts.Plugins.GetAll() {
		// FIXME: use a full-featured command parser
		scanner := bufio.NewScanner(strings.NewReader(pluginCmd))
		scanner.Split(bufio.ScanWords)
		var cmd []string
		for scanner.Scan() {
			cmd = append(cmd, scanner.Text())
		}
		if len(cmd) < 1 {
			log.Fatalf("empty plugin definition: '%s'", pluginCmd)
		}
		fmt.Printf("---> loading plugin '%s'\n", pluginCmd)
		if err := eng.Job(cmd[0], cmd[1:]...).Run(); err != nil {
			log.Fatalf("error loading plugin '%s': %s\n", pluginCmd, err)
		}
	}
	// Pass arguments as a new job
	job := eng.Job(opts.Args[0], opts.Args[1:]...)
	if err := job.Run(); err != nil {
		os.Exit(int(job.Status()))
	}
}


func showVersion() {
	fmt.Printf("Docker version %s, build %s\n", dockerversion.VERSION, dockerversion.GITCOMMIT)
}

type Opts struct {
	Version	bool
	Debug	bool
	Plugins	opts.ListOpts
	Args	[]string
}

type LegacyOpts struct {
	Daemon	bool
	Hosts	opts.ListOpts
}

func (o *Opts) Parse() {
	// Register supported flags
	flag.BoolVar(&o.Version, []string{"v", "-version"}, false, "Print version information and quit")
	flag.BoolVar(&o.Debug, []string{"D", "-debug"}, false, "Enable debug mode")
	flag.Var(&o.Plugins, []string{"-load"}, "Load a plugin")
	flag.Lookup("-load").DefValue = "'COMMAND [ARG...]'"
	// Register legacy flags
	// FIXME: legacy flags should be hidden from the usage message
	legacy := LegacyOpts{
		Hosts:	opts.NewListOpts(api.ValidateHost),
	}
	flag.BoolVar(&legacy.Daemon, []string{"d", "-daemon"}, false, "(deprecated) enable daemon mode")
	flag.Var(&legacy.Hosts, []string{"H", "-host"}, "(deprecated) listen or connect to a remote daemon")
	flag.Lookup("-host").DefValue = "'tcp://HOST[:PORT] | unix://PATH'"
	// First-pass
	flag.Parse()
	args := flag.Args()
	// -d -> legacy daemon mode
	if legacy.Daemon {
		// '-d' means 1) load daemon and rest plugins and 2) call restserver
		// plugin 'daemon' configures docker to run lxc containers itself
		o.Plugins.Set("daemon")
		o.Plugins.Set("rest")
		args = []string{"restserver"}
		for _, host := range legacy.Hosts.GetAll() {
			args = append(args, "-H", host)
		}
	// No -d + no plugins -> legacy client mode
	} else if o.Plugins.Len() == 0 {
		// no '-d' means 1) load client plugin and pass on the command unchanged
		// plugin 'client' configures docker to forward all commands
		// over a remote rest api
		var host string
		if legacy.Hosts.Len() > 0 {
			host = legacy.Hosts.GetAll()[0]
		} else {
			host = os.Getenv("DOCKER_HOST")
		}
		o.Plugins.Set("rest")
		o.Plugins.Set("restclient " + host)
	}
	o.Args = args
}
