package main

import (
	"bufio"
	"fmt"
	"log"
	"io/ioutil"
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

	var (
		flVersion	= flag.Bool([]string{"v", "-version"}, false, "Print version information and quit")
		flDebug		= flag.Bool([]string{"D", "-debug"}, false, "Enable debug mode")
		flPlugins	= opts.NewListOpts(nil)
	)

	flag.Var(&flPlugins, []string{"-plugin"}, "PLUGIN [ARG...]")
	flag.Parse()

	if *flVersion {
		showVersion()
		return
	}

	if *flDebug {
		os.Setenv("DEBUG", "1")
	}
	args := parseLegacy(&flPlugins, flag.Args()...)
	eng, err := engine.New(".docker")
	if err != nil {
		log.Fatal(err)
	}
	// Register builtins
	builtins.Register(eng)
	// Load plugins
	for _, pluginCmd := range flPlugins.GetAll() {
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
	job := eng.Job(args[0], args[1:]...)
	if err := job.Run(); err != nil {
		os.Exit(int(job.Status()))
	}
}


func parseLegacy(plugins *opts.ListOpts, argsIn ...string) (argsOut []string) {
	cmd := flag.NewFlagSet("extract-legacy-plugins", flag.ContinueOnError)
	cmd.SetOutput(ioutil.Discard)
	flDaemon := cmd.Bool([]string{"d", "-daemon"}, false, "Enable daemon mode")
	flHosts := opts.NewListOpts(api.ValidateHost)
	cmd.Var(&flHosts, []string{"-H", "-host"}, "")
	cmd.Parse(argsIn)
	argsOut = cmd.Args()
	if *flDaemon {
		// '-d' means 1) load daemon plugin and 2) call serveapi
		// plugin 'daemon' configures docker to run lxc containers itself
		plugins.Set("daemon")
		plugins.Set("rest")
		argsOut = []string{"restserver"}
		for _, host := range flHosts.GetAll() {
			argsOut = append(argsOut, "-H", host)
		}
	} else if plugins.Len() == 0 {
		// no '-d' means 1) load client plugin and pass on the command unchanged
		// plugin 'client' configures docker to forward all commands
		// over a remote rest api
		var host string
		if flHosts.Len() > 0 {
			host = flHosts.GetAll()[0]
		} else {
			host = os.Getenv("DOCKER_HOST")
		}
		plugins.Set("client " + host)
	}
	return argsOut
}

func showVersion() {
	fmt.Printf("Docker version %s, build %s\n", dockerversion.VERSION, dockerversion.GITCOMMIT)
}
