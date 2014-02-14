package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dotcloud/docker/auth"
	"github.com/dotcloud/docker/dockerversion"
	"github.com/dotcloud/docker/engine"
	flag "github.com/dotcloud/docker/pkg/mflag"
	"github.com/dotcloud/docker/pkg/term"
	"github.com/dotcloud/docker/utils"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"syscall"
)

var (
	ErrConnectionRefused = errors.New("Can't connect to docker daemon. Is 'docker -d' running on this host?")
)

func (cli *DockerCli) getMethod(name string) (func(*engine.Job) engine.Status, bool) {
	methodName := "Cmd" + strings.ToUpper(name[:1]) + strings.ToLower(name[1:])
	method := reflect.ValueOf(cli).MethodByName(methodName)
	if !method.IsValid() {
		return nil, false
	}
	return method.Interface().(func(*engine.Job) engine.Status), true
}

func ConnectApi(job *engine.Job) engine.Status {
	if len(job.Args) != 1 {
		return job.Errorf("Usage: %s URL", job.Name)
	}
	peer, err := url.Parse(job.Args[0])
	if err != nil {
		return job.Errorf("invalid url: %s", job.Args[0])
	}
	cli := NewDockerCli(os.Stdin, os.Stdout, os.Stderr, peer.Scheme, peer.Host)
	for _, command := range []string{
		"attach",
		"build",
		"commit",
		"cp",
		"diff",
		"events",
		"export",
		"history",
		"images",
		"import",
		"info",
		"insert",
		"inspect",
		"kill",
		"load",
		"login",
		"logs",
		"port",
		"ps",
		"pull",
		"push",
		"restart",
		"rm",
		"rmi",
		"run",
		"save",
		"search",
		"start",
		"stop",
		"tag",
		"top",
		"version",
		"wait",
	} {
		method, ok := cli.getMethod(command)
		if !ok {
			return job.Errorf("no handler for command '%s'", command)
		}
		job.Eng.Register(command, method)
	}
	// FIXME: register fallback handler to print full usage on wrong command.
	// (this is the 0.8 behavior)
	return engine.StatusOK
}


func (cli *DockerCli) forwardAllSignals(cid string) chan os.Signal {
	sigc := make(chan os.Signal, 1)
	utils.CatchAll(sigc)
	go func() {
		for s := range sigc {
			if s == syscall.SIGCHLD {
				continue
			}
			if _, _, err := readBody(cli.call("POST", fmt.Sprintf("/containers/%s/kill?signal=%d", cid, s), nil, false)); err != nil {
				utils.Debugf("Error sending signal: %s", err)
			}
		}
	}()
	return sigc
}

func (cli *DockerCli) WalkTree(noTrunc bool, images *engine.Table, byParent map[string]*engine.Table, prefix string, printNode func(cli *DockerCli, noTrunc bool, image *engine.Env, prefix string)) {
	length := images.Len()
	if length > 1 {
		for index, image := range images.Data {
			if index+1 == length {
				printNode(cli, noTrunc, image, prefix+"└─")
				if subimages, exists := byParent[image.Get("Id")]; exists {
					cli.WalkTree(noTrunc, subimages, byParent, prefix+"  ", printNode)
				}
			} else {
				printNode(cli, noTrunc, image, prefix+"\u251C─")
				if subimages, exists := byParent[image.Get("Id")]; exists {
					cli.WalkTree(noTrunc, subimages, byParent, prefix+"\u2502 ", printNode)
				}
			}
		}
	} else {
		for _, image := range images.Data {
			printNode(cli, noTrunc, image, prefix+"└─")
			if subimages, exists := byParent[image.Get("Id")]; exists {
				cli.WalkTree(noTrunc, subimages, byParent, prefix+"  ", printNode)
			}
		}
	}
}

func (cli *DockerCli) printVizNode(noTrunc bool, image *engine.Env, prefix string) {
	var (
		imageID  string
		parentID string
	)
	if noTrunc {
		imageID = image.Get("Id")
		parentID = image.Get("ParentId")
	} else {
		imageID = utils.TruncateID(image.Get("Id"))
		parentID = utils.TruncateID(image.Get("ParentId"))
	}
	if parentID == "" {
		fmt.Fprintf(cli.out, " base -> \"%s\" [style=invis]\n", imageID)
	} else {
		fmt.Fprintf(cli.out, " \"%s\" -> \"%s\"\n", parentID, imageID)
	}
	if image.GetList("RepoTags")[0] != "<none>:<none>" {
		fmt.Fprintf(cli.out, " \"%s\" [label=\"%s\\n%s\",shape=box,fillcolor=\"paleturquoise\",style=\"filled,rounded\"];\n",
			imageID, imageID, strings.Join(image.GetList("RepoTags"), "\\n"))
	}
}

func (cli *DockerCli) printTreeNode(noTrunc bool, image *engine.Env, prefix string) {
	var imageID string
	if noTrunc {
		imageID = image.Get("Id")
	} else {
		imageID = utils.TruncateID(image.Get("Id"))
	}

	fmt.Fprintf(cli.out, "%s%s Virtual Size: %s", prefix, imageID, utils.HumanSize(image.GetInt64("VirtualSize")))
	if image.GetList("RepoTags")[0] != "<none>:<none>" {
		fmt.Fprintf(cli.out, " Tags: %s\n", strings.Join(image.GetList("RepoTags"), ", "))
	} else {
		fmt.Fprint(cli.out, "\n")
	}
}


func (cli *DockerCli) call(method, path string, data interface{}, passAuthInfo bool) (io.ReadCloser, int, error) {
	params := bytes.NewBuffer(nil)
	if data != nil {
		if env, ok := data.(engine.Env); ok {
			if err := env.Encode(params); err != nil {
				return nil, -1, err
			}
		} else {
			buf, err := json.Marshal(data)
			if err != nil {
				return nil, -1, err
			}
			if _, err := params.Write(buf); err != nil {
				return nil, -1, err
			}
		}
	}
	// fixme: refactor client to support redirect
	re := regexp.MustCompile("/+")
	path = re.ReplaceAllString(path, "/")

	req, err := http.NewRequest(method, fmt.Sprintf("/v%g%s", APIVERSION, path), params)
	if err != nil {
		return nil, -1, err
	}
	if passAuthInfo {
		cli.LoadConfigFile()
		// Resolve the Auth config relevant for this server
		authConfig := cli.configFile.ResolveAuthConfig(auth.IndexServerAddress())
		getHeaders := func(authConfig auth.AuthConfig) (map[string][]string, error) {
			buf, err := json.Marshal(authConfig)
			if err != nil {
				return nil, err
			}
			registryAuthHeader := []string{
				base64.URLEncoding.EncodeToString(buf),
			}
			return map[string][]string{"X-Registry-Auth": registryAuthHeader}, nil
		}
		if headers, err := getHeaders(authConfig); err == nil && headers != nil {
			for k, v := range headers {
				req.Header[k] = v
			}
		}
	}
	req.Header.Set("User-Agent", "Docker-Client/"+dockerversion.VERSION)
	req.Host = cli.addr
	if data != nil {
		req.Header.Set("Content-Type", "application/json")
	} else if method == "POST" {
		req.Header.Set("Content-Type", "plain/text")
	}
	dial, err := net.Dial(cli.proto, cli.addr)
	if err != nil {
		if strings.Contains(err.Error(), "connection refused") {
			return nil, -1, ErrConnectionRefused
		}
		return nil, -1, err
	}
	clientconn := httputil.NewClientConn(dial, nil)
	resp, err := clientconn.Do(req)
	if err != nil {
		clientconn.Close()
		if strings.Contains(err.Error(), "connection refused") {
			return nil, -1, ErrConnectionRefused
		}
		return nil, -1, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, -1, err
		}
		if len(body) == 0 {
			return nil, resp.StatusCode, fmt.Errorf("Error :%s", http.StatusText(resp.StatusCode))
		}
		return nil, resp.StatusCode, fmt.Errorf("Error: %s", bytes.TrimSpace(body))
	}

	wrapper := utils.NewReadCloserWrapper(resp.Body, func() error {
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		return clientconn.Close()
	})
	return wrapper, resp.StatusCode, nil
}

func (cli *DockerCli) stream(method, path string, in io.Reader, out io.Writer, headers map[string][]string) error {
	if (method == "POST" || method == "PUT") && in == nil {
		in = bytes.NewReader([]byte{})
	}

	// fixme: refactor client to support redirect
	re := regexp.MustCompile("/+")
	path = re.ReplaceAllString(path, "/")

	req, err := http.NewRequest(method, fmt.Sprintf("/v%g%s", APIVERSION, path), in)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Docker-Client/"+dockerversion.VERSION)
	req.Host = cli.addr
	if method == "POST" {
		req.Header.Set("Content-Type", "plain/text")
	}

	if headers != nil {
		for k, v := range headers {
			req.Header[k] = v
		}
	}

	dial, err := net.Dial(cli.proto, cli.addr)
	if err != nil {
		if strings.Contains(err.Error(), "connection refused") {
			return fmt.Errorf("Can't connect to docker daemon. Is 'docker -d' running on this host?")
		}
		return err
	}
	clientconn := httputil.NewClientConn(dial, nil)
	resp, err := clientconn.Do(req)
	defer clientconn.Close()
	if err != nil {
		if strings.Contains(err.Error(), "connection refused") {
			return fmt.Errorf("Can't connect to docker daemon. Is 'docker -d' running on this host?")
		}
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		if len(body) == 0 {
			return fmt.Errorf("Error :%s", http.StatusText(resp.StatusCode))
		}
		return fmt.Errorf("Error: %s", bytes.TrimSpace(body))
	}

	if MatchesContentType(resp.Header.Get("Content-Type"), "application/json") {
		return utils.DisplayJSONMessagesStream(resp.Body, out, cli.terminalFd, cli.isTerminal)
	}
	if _, err := io.Copy(out, resp.Body); err != nil {
		return err
	}
	return nil
}

func (cli *DockerCli) hijack(method, path string, setRawTerminal bool, in io.ReadCloser, stdout, stderr io.Writer, started chan io.Closer) error {
	defer func() {
		if started != nil {
			close(started)
		}
	}()
	// fixme: refactor client to support redirect
	re := regexp.MustCompile("/+")
	path = re.ReplaceAllString(path, "/")

	req, err := http.NewRequest(method, fmt.Sprintf("/v%g%s", APIVERSION, path), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Docker-Client/"+dockerversion.VERSION)
	req.Header.Set("Content-Type", "plain/text")
	req.Host = cli.addr

	dial, err := net.Dial(cli.proto, cli.addr)
	if err != nil {
		if strings.Contains(err.Error(), "connection refused") {
			return fmt.Errorf("Can't connect to docker daemon. Is 'docker -d' running on this host?")
		}
		return err
	}
	clientconn := httputil.NewClientConn(dial, nil)
	defer clientconn.Close()

	// Server hijacks the connection, error 'connection closed' expected
	clientconn.Do(req)

	rwc, br := clientconn.Hijack()
	defer rwc.Close()

	if started != nil {
		started <- rwc
	}

	var receiveStdout chan error

	var oldState *term.State

	if in != nil && setRawTerminal && cli.isTerminal && os.Getenv("NORAW") == "" {
		oldState, err = term.SetRawTerminal(cli.terminalFd)
		if err != nil {
			return err
		}
		defer term.RestoreTerminal(cli.terminalFd, oldState)
	}

	if stdout != nil || stderr != nil {
		receiveStdout = utils.Go(func() (err error) {
			defer func() {
				if in != nil {
					if setRawTerminal && cli.isTerminal {
						term.RestoreTerminal(cli.terminalFd, oldState)
					}
					in.Close()
				}
			}()

			// When TTY is ON, use regular copy
			if setRawTerminal {
				_, err = io.Copy(stdout, br)
			} else {
				_, err = utils.StdCopy(stdout, stderr, br)
			}
			utils.Debugf("[hijack] End of stdout")
			return err
		})
	}

	sendStdin := utils.Go(func() error {
		if in != nil {
			io.Copy(rwc, in)
			utils.Debugf("[hijack] End of stdin")
		}
		if tcpc, ok := rwc.(*net.TCPConn); ok {
			if err := tcpc.CloseWrite(); err != nil {
				utils.Errorf("Couldn't send EOF: %s\n", err)
			}
		} else if unixc, ok := rwc.(*net.UnixConn); ok {
			if err := unixc.CloseWrite(); err != nil {
				utils.Errorf("Couldn't send EOF: %s\n", err)
			}
		}
		// Discard errors due to pipe interruption
		return nil
	})

	if stdout != nil || stderr != nil {
		if err := <-receiveStdout; err != nil {
			utils.Errorf("Error receiveStdout: %s", err)
			return err
		}
	}

	if !cli.isTerminal {
		if err := <-sendStdin; err != nil {
			utils.Errorf("Error sendStdin: %s", err)
			return err
		}
	}
	return nil

}

func (cli *DockerCli) getTtySize() (int, int) {
	if !cli.isTerminal {
		return 0, 0
	}
	ws, err := term.GetWinsize(cli.terminalFd)
	if err != nil {
		utils.Errorf("Error getting size: %s", err)
		if ws == nil {
			return 0, 0
		}
	}
	return int(ws.Height), int(ws.Width)
}

func (cli *DockerCli) resizeTty(id string) {
	height, width := cli.getTtySize()
	if height == 0 && width == 0 {
		return
	}
	v := url.Values{}
	v.Set("h", strconv.Itoa(height))
	v.Set("w", strconv.Itoa(width))
	if _, _, err := readBody(cli.call("POST", "/containers/"+id+"/resize?"+v.Encode(), nil, false)); err != nil {
		utils.Errorf("Error resize: %s", err)
	}
}

func (cli *DockerCli) monitorTtySize(id string) error {
	cli.resizeTty(id)

	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, syscall.SIGWINCH)
	go func() {
		for _ = range sigchan {
			cli.resizeTty(id)
		}
	}()
	return nil
}

func (cli *DockerCli) Subcmd(name, signature, description string) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.Usage = func() {
		fmt.Fprintf(cli.err, "\nUsage: docker %s %s\n\n%s\n\n", name, signature, description)
		flags.PrintDefaults()
		os.Exit(2)
	}
	return flags
}

func (cli *DockerCli) LoadConfigFile() (err error) {
	cli.configFile, err = auth.LoadConfig(os.Getenv("HOME"))
	if err != nil {
		fmt.Fprintf(cli.err, "WARNING: %s\n", err)
	}
	return err
}

func waitForExit(cli *DockerCli, containerId string) (int, error) {
	stream, _, err := cli.call("POST", "/containers/"+containerId+"/wait", nil, false)
	if err != nil {
		return -1, err
	}

	var out engine.Env
	if err := out.Decode(stream); err != nil {
		return -1, err
	}
	return out.GetInt("StatusCode"), nil
}

// getExitCode perform an inspect on the container. It returns
// the running state and the exit code.
func getExitCode(cli *DockerCli, containerId string) (bool, int, error) {
	body, _, err := readBody(cli.call("GET", "/containers/"+containerId+"/json", nil, false))
	if err != nil {
		// If we can't connect, then the daemon probably died.
		if err != ErrConnectionRefused {
			return false, -1, err
		}
		return false, -1, nil
	}
	c := &Container{}
	if err := json.Unmarshal(body, c); err != nil {
		return false, -1, err
	}
	return c.State.Running, c.State.ExitCode, nil
}

func readBody(stream io.ReadCloser, statusCode int, err error) ([]byte, int, error) {
	if stream != nil {
		defer stream.Close()
	}
	if err != nil {
		return nil, statusCode, err
	}
	body, err := ioutil.ReadAll(stream)
	if err != nil {
		return nil, -1, err
	}
	return body, statusCode, nil
}

func NewDockerCli(in io.ReadCloser, out, err io.Writer, proto, addr string) *DockerCli {
	var (
		isTerminal = false
		terminalFd uintptr
	)

	if in != nil {
		if file, ok := in.(*os.File); ok {
			terminalFd = file.Fd()
			isTerminal = term.IsTerminal(terminalFd)
		}
	}

	if err == nil {
		err = out
	}
	return &DockerCli{
		proto:      proto,
		addr:       addr,
		in:         in,
		out:        out,
		err:        err,
		isTerminal: isTerminal,
		terminalFd: terminalFd,
	}
}

type DockerCli struct {
	proto      string
	addr       string
	configFile *auth.ConfigFile
	in         io.ReadCloser
	out        io.Writer
	err        io.Writer
	isTerminal bool
	terminalFd uintptr
}
