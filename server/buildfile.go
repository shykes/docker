package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dotcloud/docker/archive"
	"github.com/dotcloud/docker/nat"
	"github.com/dotcloud/docker/registry"
	"github.com/dotcloud/docker/runconfig"
	"github.com/dotcloud/docker/runtime"
	"github.com/dotcloud/docker/utils"
	"github.com/dotcloud/docker/engine"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	ErrDockerfileEmpty = errors.New("Dockerfile cannot be empty")
)

type BuildFile interface {
	Build(io.Reader) (string, error)
	CmdFrom(string) error
	CmdRun(string) error
}

type buildFile struct {
	eng	*engine.Engine
	runtime *runtime.Runtime
	srv     *Server

	image      string
	maintainer string
	config     *runconfig.Config

	contextPath string
	context     *utils.TarSum

	verbose      bool
	utilizeCache bool
	rm           bool

	authConfig *registry.AuthConfig
	configFile *registry.ConfigFile

	tmpContainers map[string]struct{}
	tmpImages     map[string]struct{}

	outStream io.Writer
	errStream io.Writer

	// Deprecated, original writer used for ImagePull. To be removed.
	outOld io.Writer
	sf     *utils.StreamFormatter
}

func (b *buildFile) clearTmp(containers map[string]struct{}) {
	for c := range containers {
		if err := b.eng.Job("destroy", c).Run(); err != nil {
			fmt.Fprintf(b.outStream, "Error removing intermediate container %s: %s\n", utils.TruncateID(c), err.Error())
		} else {
			delete(containers, c)
			fmt.Fprintf(b.outStream, "Removed intermediate container %s\n", utils.TruncateID(c))
		}
	}
}

func (b *buildFile) CmdFrom(name string) error {
	// YOU ARE HERE: convert this to jobs
	lookup := b.eng.Job("getimage", name)
	lookup.SetenvBool("autopull", true ) // auto-pull the image if it doesn't exist.
	lookup.SetenvJson("auth", b.configFile)
	img, err := lookup.Stdout.AddEnv()
	if err != nil {
		return err
	}
	if err := lookup.Run(); err != nil {
		return err
	}
	b.image = img.Get("id")
	if img.Exists("config") {
		b.config = &runconfig.Config{}
		img.GetJson("config", b.config)
	}
	if b.config.Env == nil || len(b.config.Env) == 0 {
		// FIXME: it doesn't feel right accessing runtime-specific values like DefaultPathEnv
		// in 'docker build'.
		b.config.Env = append(b.config.Env, "HOME=/", "PATH="+runtime.DefaultPathEnv)
	}
	// Process ONBUILD triggers if they exist
	if nTriggers := len(b.config.OnBuild); nTriggers != 0 {
		fmt.Fprintf(b.errStream, "# Executing %d build triggers\n", nTriggers)
	}
	for n, step := range b.config.OnBuild {
		splitStep := strings.Split(step, " ")
		stepInstruction := strings.ToUpper(strings.Trim(splitStep[0], " "))
		switch stepInstruction {
		case "ONBUILD":
			return fmt.Errorf("Source image contains forbidden chained `ONBUILD ONBUILD` trigger: %s", step)
		case "MAINTAINER", "FROM":
			return fmt.Errorf("Source image contains forbidden %s trigger: %s", stepInstruction, step)
		}
		if err := b.BuildStep(fmt.Sprintf("onbuild-%d", n), step); err != nil {
			return err
		}
	}
	b.config.OnBuild = []string{}
	return nil
}

// The ONBUILD command declares a build instruction to be executed in any future build
// using the current image as a base.
func (b *buildFile) CmdOnbuild(trigger string) error {
	splitTrigger := strings.Split(trigger, " ")
	triggerInstruction := strings.ToUpper(strings.Trim(splitTrigger[0], " "))
	switch triggerInstruction {
	case "ONBUILD":
		return fmt.Errorf("Chaining ONBUILD via `ONBUILD ONBUILD` isn't allowed")
	case "MAINTAINER", "FROM":
		return fmt.Errorf("%s isn't allowed as an ONBUILD trigger", triggerInstruction)
	}
	b.config.OnBuild = append(b.config.OnBuild, trigger)
	return b.commit("", b.config.Cmd, fmt.Sprintf("ONBUILD %s", trigger))
}

func (b *buildFile) CmdMaintainer(name string) error {
	b.maintainer = name
	return b.commit("", b.config.Cmd, fmt.Sprintf("MAINTAINER %s", name))
}

// probeCache checks to see if image-caching is enabled (`b.utilizeCache`)
// and if so attempts to look up the current `b.image` and `b.config` pair
// in the current server `b.srv`. If an image is found, probeCache returns
// `(true, nil)`. If no image is found, it returns `(false, nil)`. If there
// is any error, it returns `(false, err)`.
func (b *buildFile) probeCache() (bool, error) {
	if b.utilizeCache {
		getcached := b.eng.Job("image_byparent", b.image)
		getcached.SetenvJson("config", b.config)
		var cacheID string
		getcached.Stdout.AddString(&cacheID)
		if err := getcached.Run(); err != nil {
			return false, err
		} else if cacheID != "" {
			fmt.Fprintf(b.outStream, " ---> Using cache\n")
			utils.Debugf("[BUILDER] Use cached version")
			b.image = cacheID
			return true, nil
		} else {
			utils.Debugf("[BUILDER] Cache miss")
		}
	}
	return false, nil
}

func (b *buildFile) CmdRun(args string) error {
	if b.image == "" {
		return fmt.Errorf("Please provide a source image with `from` prior to run")
	}
	config, _, _, err := runconfig.Parse(append([]string{b.image}, b.buildCmdFromJson(args)...), nil)
	if err != nil {
		return err
	}

	cmd := b.config.Cmd
	b.config.Cmd = nil
	runconfig.Merge(b.config, config)

	defer func(cmd []string) { b.config.Cmd = cmd }(cmd)

	utils.Debugf("Command to be executed: %v", b.config.Cmd)

	hit, err := b.probeCache()
	if err != nil {
		return err
	}
	if hit {
		return nil
	}

	// Create the container
	create := b.eng.Job("create")
	var id string
	create.Stdout.AddString(&id)
	create.Stderr.Add(b.outStream)
	if err := create.Env().Import(b.config); err != nil {
		return err
	}
	if err := create.Run(); err != nil {
		return err
	}
	// override the entry point that may have been picked up from the base image
	// FIXME: this is a hack to workaround the brittle "mergeconfig" logic.
	// The solution is to get rid of mergeconfig altogether, in favor of explicit
	// changes using the Dockerfile syntax.
	if err := b.eng.Job("cmd", append([]string{id}, b.config.Cmd...)...).Run(); err != nil {
		return err
	}
	b.tmpContainers[id] = struct{}{}
	// Attach to the container in verbose mode
	if b.verbose {
		attach := b.eng.Job("attach", id)
		attach.SetenvBool("stream", true)
		attach.SetenvBool("stdout", true)
		attach.Stdout.Add(b.outStream)
		attach.SetenvBool("stderr", true)
		attach.Stderr.Add(b.errStream)
	}
	// Start the container
	if err := b.eng.Job("start", id).Run(); err != nil {
		return err
	}
	// Wait for the container to finish
	// FIXME: this is racy, because the container could have been start/stopped
	// any number of times from another caller in between our "start" and "wait"
	// calls. We need an atomic start+wait call.
	var status string
	wait := b.eng.Job("wait", id)
	wait.Stdout.AddString(&status)
	if err := wait.Run(); err != nil {
		return err
	}
	if status != "0" {
		// FIXME: why do we need this weird custom error? -- Solomon
		ret, err := strconv.ParseInt(status, 10, 32)
		if err != nil {
			ret = 1
		}
		return &utils.JSONError{
			Message: fmt.Sprintf("The command %v returned a non-zero code: %d", b.config.Cmd, ret),
			Code:    int(ret),
		}
	}
	// Commit the image
	if err := b.commit(id, cmd, "run"); err != nil {
		return err
	}

	return nil
}

func (b *buildFile) FindEnvKey(key string) int {
	for k, envVar := range b.config.Env {
		envParts := strings.SplitN(envVar, "=", 2)
		if key == envParts[0] {
			return k
		}
	}
	return -1
}

func (b *buildFile) ReplaceEnvMatches(value string) (string, error) {
	exp, err := regexp.Compile("(\\\\\\\\+|[^\\\\]|\\b|\\A)\\$({?)([[:alnum:]_]+)(}?)")
	if err != nil {
		return value, err
	}
	matches := exp.FindAllString(value, -1)
	for _, match := range matches {
		match = match[strings.Index(match, "$"):]
		matchKey := strings.Trim(match, "${}")

		for _, envVar := range b.config.Env {
			envParts := strings.SplitN(envVar, "=", 2)
			envKey := envParts[0]
			envValue := envParts[1]

			if envKey == matchKey {
				value = strings.Replace(value, match, envValue, -1)
				break
			}
		}
	}
	return value, nil
}

func (b *buildFile) CmdEnv(args string) error {
	tmp := strings.SplitN(args, " ", 2)
	if len(tmp) != 2 {
		return fmt.Errorf("Invalid ENV format")
	}
	key := strings.Trim(tmp[0], " \t")
	value := strings.Trim(tmp[1], " \t")

	envKey := b.FindEnvKey(key)
	replacedValue, err := b.ReplaceEnvMatches(value)
	if err != nil {
		return err
	}
	replacedVar := fmt.Sprintf("%s=%s", key, replacedValue)

	if envKey >= 0 {
		b.config.Env[envKey] = replacedVar
	} else {
		b.config.Env = append(b.config.Env, replacedVar)
	}
	return b.commit("", b.config.Cmd, fmt.Sprintf("ENV %s", replacedVar))
}

func (b *buildFile) buildCmdFromJson(args string) []string {
	var cmd []string
	if err := json.Unmarshal([]byte(args), &cmd); err != nil {
		utils.Debugf("Error unmarshalling: %s, setting to /bin/sh -c", err)
		cmd = []string{"/bin/sh", "-c", args}
	}
	return cmd
}

func (b *buildFile) CmdCmd(args string) error {
	cmd := b.buildCmdFromJson(args)
	b.config.Cmd = cmd
	if err := b.commit("", b.config.Cmd, fmt.Sprintf("CMD %v", cmd)); err != nil {
		return err
	}
	return nil
}

func (b *buildFile) CmdEntrypoint(args string) error {
	entrypoint := b.buildCmdFromJson(args)
	b.config.Entrypoint = entrypoint
	if err := b.commit("", b.config.Cmd, fmt.Sprintf("ENTRYPOINT %v", entrypoint)); err != nil {
		return err
	}
	return nil
}

func (b *buildFile) CmdExpose(args string) error {
	portsTab := strings.Split(args, " ")

	if b.config.ExposedPorts == nil {
		b.config.ExposedPorts = make(nat.PortSet)
	}
	ports, _, err := nat.ParsePortSpecs(append(portsTab, b.config.PortSpecs...))
	if err != nil {
		return err
	}
	for port := range ports {
		if _, exists := b.config.ExposedPorts[port]; !exists {
			b.config.ExposedPorts[port] = struct{}{}
		}
	}
	b.config.PortSpecs = nil

	return b.commit("", b.config.Cmd, fmt.Sprintf("EXPOSE %v", ports))
}

func (b *buildFile) CmdUser(args string) error {
	b.config.User = args
	return b.commit("", b.config.Cmd, fmt.Sprintf("USER %v", args))
}

func (b *buildFile) CmdInsert(args string) error {
	return fmt.Errorf("INSERT has been deprecated. Please use ADD instead")
}

func (b *buildFile) CmdCopy(args string) error {
	return fmt.Errorf("COPY has been deprecated. Please use ADD instead")
}

func (b *buildFile) CmdWorkdir(workdir string) error {
	if workdir[0] == '/' {
		b.config.WorkingDir = workdir
	} else {
		if b.config.WorkingDir == "" {
			b.config.WorkingDir = "/"
		}
		b.config.WorkingDir = filepath.Join(b.config.WorkingDir, workdir)
	}
	return b.commit("", b.config.Cmd, fmt.Sprintf("WORKDIR %v", workdir))
}

func (b *buildFile) CmdVolume(args string) error {
	if args == "" {
		return fmt.Errorf("Volume cannot be empty")
	}

	var volume []string
	if err := json.Unmarshal([]byte(args), &volume); err != nil {
		volume = []string{args}
	}
	if b.config.Volumes == nil {
		b.config.Volumes = map[string]struct{}{}
	}
	for _, v := range volume {
		b.config.Volumes[v] = struct{}{}
	}
	if err := b.commit("", b.config.Cmd, fmt.Sprintf("VOLUME %s", args)); err != nil {
		return err
	}
	return nil
}

func (b *buildFile) checkPathForAddition(orig string) error {
	origPath := path.Join(b.contextPath, orig)
	if p, err := filepath.EvalSymlinks(origPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s: no such file or directory", orig)
		}
		return err
	} else {
		origPath = p
	}
	if !strings.HasPrefix(origPath, b.contextPath) {
		return fmt.Errorf("Forbidden path outside the build context: %s (%s)", orig, origPath)
	}
	_, err := os.Stat(origPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s: no such file or directory", orig)
		}
		return err
	}
	return nil
}

// FIXME: extract the container-facing part to a job?
// Job("fs_untar", containername, despath)
//	-> tar stream in stdin
//	-> mode + ownership in env
// Or if we dont want to untar:
// Job("fs_write", containername, destpath) (content
//	-> file content in stdin
//	-> mode + ownership in env
func (b *buildFile) addContext(container *runtime.Container, orig, dest string, remote bool) error {

	add := eng.Job("add", id, dest)

	var (
		err      error
		origPath = path.Join(b.contextPath, orig)
	)

	if destPath != container.RootfsPath() {
		destPath, err = fs.FollowSymlinkInScope(destPath, container.RootfsPath())
		if err != nil {
			return err
		}
	}

	// Preserve the trailing '/'
	if strings.HasSuffix(dest, "/") {
		destPath = destPath + "/"
	}
	fi, err := os.Stat(origPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s: no such file or directory", orig)
		}
		return err
	}

	chownR := func(destPath string, uid, gid int) error {
		return filepath.Walk(destPath, func(path string, info os.FileInfo, err error) error {
			if err := os.Lchown(path, uid, gid); err != nil {
				return err
			}
			return nil
		})
	}

	if fi.IsDir() {
		if err := archive.CopyWithTar(origPath, destPath); err != nil {
			return err
		}
		if err := chownR(destPath, 0, 0); err != nil {
			return err
		}
		return nil
	}


	// -> YOU ARE HERE

	// First try to unpack the source as an archive
	// to support the untar feature we need to clean up the path a little bit
	// because tar is very forgiving.  First we need to strip off the archive's
	// filename from the path but this is only added if it does not end in / .
	tarDest := destPath
	if strings.HasSuffix(tarDest, "/") {
		tarDest = filepath.Dir(destPath)
	}

	// If we are adding a remote file, do not try to untar it
	if !remote {
		// try to successfully untar the orig
		if err := archive.UntarPath(origPath, tarDest); err == nil {
			return nil
		}
		utils.Debugf("Couldn't untar %s to %s: %s", origPath, destPath, err)
	}

	// If that fails, just copy it as a regular file
	// but do not use all the magic path handling for the tar path
	if err := os.MkdirAll(path.Dir(destPath), 0755); err != nil {
		return err
	}
	if err := archive.CopyWithTar(origPath, destPath); err != nil {
		return err
	}

	if err := chownR(destPath, 0, 0); err != nil {
		return err
	}
	return nil
}

func (b *buildFile) CmdAdd(args string) error {
	if b.context == nil {
		return fmt.Errorf("No context given. Impossible to use ADD")
	}
	tmp := strings.SplitN(args, " ", 2)
	if len(tmp) != 2 {
		return fmt.Errorf("Invalid ADD format")
	}

	orig, err := b.ReplaceEnvMatches(strings.Trim(tmp[0], " \t"))
	if err != nil {
		return err
	}

	dest, err := b.ReplaceEnvMatches(strings.Trim(tmp[1], " \t"))
	if err != nil {
		return err
	}

	cmd := b.config.Cmd
	b.config.Cmd = []string{"/bin/sh", "-c", fmt.Sprintf("#(nop) ADD %s in %s", orig, dest)}
	b.config.Image = b.image

	var (
		origPath   = orig
		destPath   = dest
		remoteHash string
		isRemote   bool
	)

	if utils.IsURL(orig) {
		// Initiate the download
		isRemote = true
		resp, err := utils.Download(orig)
		if err != nil {
			return err
		}

		// Create a tmp dir
		tmpDirName, err := ioutil.TempDir(b.contextPath, "docker-remote")
		if err != nil {
			return err
		}

		// Create a tmp file within our tmp dir
		tmpFileName := path.Join(tmpDirName, "tmp")
		tmpFile, err := os.OpenFile(tmpFileName, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmpDirName)

		// Download and dump result to tmp file
		if _, err := io.Copy(tmpFile, resp.Body); err != nil {
			tmpFile.Close()
			return err
		}
		tmpFile.Close()

		origPath = path.Join(filepath.Base(tmpDirName), filepath.Base(tmpFileName))

		// Process the checksum
		r, err := archive.Tar(tmpFileName, archive.Uncompressed)
		if err != nil {
			return err
		}
		tarSum := utils.TarSum{Reader: r, DisableCompression: true}
		remoteHash = tarSum.Sum(nil)
		r.Close()

		// If the destination is a directory, figure out the filename.
		if strings.HasSuffix(dest, "/") {
			u, err := url.Parse(orig)
			if err != nil {
				return err
			}
			path := u.Path
			if strings.HasSuffix(path, "/") {
				path = path[:len(path)-1]
			}
			parts := strings.Split(path, "/")
			filename := parts[len(parts)-1]
			if filename == "" {
				return fmt.Errorf("cannot determine filename from url: %s", u)
			}
			destPath = dest + filename
		}
	}

	if err := b.checkPathForAddition(origPath); err != nil {
		return err
	}

	// Hash path and check the cache
	if b.utilizeCache {
		var (
			hash string
			sums = b.context.GetSums()
		)

		if remoteHash != "" {
			hash = remoteHash
		} else if fi, err := os.Stat(path.Join(b.contextPath, origPath)); err != nil {
			return err
		} else if fi.IsDir() {
			var subfiles []string
			for file, sum := range sums {
				absFile := path.Join(b.contextPath, file)
				absOrigPath := path.Join(b.contextPath, origPath)
				if strings.HasPrefix(absFile, absOrigPath) {
					subfiles = append(subfiles, sum)
				}
			}
			sort.Strings(subfiles)
			hasher := sha256.New()
			hasher.Write([]byte(strings.Join(subfiles, ",")))
			hash = "dir:" + hex.EncodeToString(hasher.Sum(nil))
		} else {
			if origPath[0] == '/' && len(origPath) > 1 {
				origPath = origPath[1:]
			}
			origPath = strings.TrimPrefix(origPath, "./")
			if h, ok := sums[origPath]; ok {
				hash = "file:" + h
			}
		}
		b.config.Cmd = []string{"/bin/sh", "-c", fmt.Sprintf("#(nop) ADD %s in %s", hash, dest)}
		hit, err := b.probeCache()
		if err != nil {
			return err
		}
		// If we do not have a hash, never use the cache
		if hit && hash != "" {
			return nil
		}
	}

	// Create the container and start it
	create := job.Eng.Job("create")
	if err := create.Env().Import(b.config); err != nil {
		return err
	}
	if err := job.Run(); err != nil {
		return err
	}
	b.tmpContainers[container.ID] = struct{}{}


	//// --> YOU ARE HERE
	// Does addContext depend on runtime or srv? How do we convert it to a job?
	if err := b.addContext(container, origPath, destPath, isRemote); err != nil {
		return err
	}

	if err := b.commit(container.ID, cmd, fmt.Sprintf("ADD %s in %s", orig, dest)); err != nil {
		return err
	}
	b.config.Cmd = cmd
	return nil
}

func (b *buildFile) create() (*runtime.Container, error) {
	if b.image == "" {
		return nil, fmt.Errorf("Please provide a source image with `from` prior to run")
	}
	b.config.Image = b.image

	// Create the container and start it
	c, _, err := b.runtime.Create(b.config, "")
	if err != nil {
		return nil, err
	}
	b.tmpContainers[c.ID] = struct{}{}
	fmt.Fprintf(b.outStream, " ---> Running in %s\n", utils.TruncateID(c.ID))

	// override the entry point that may have been picked up from the base image
	c.Path = b.config.Cmd[0]
	c.Args = b.config.Cmd[1:]

	return c, nil
}

func (b *buildFile) run(c *runtime.Container) error {
	var errCh chan error

	if b.verbose {
		errCh = utils.Go(func() error {
			return <-c.Attach(nil, nil, b.outStream, b.errStream)
		})
	}

	//start the container
	if err := c.Start(); err != nil {
		return err
	}

	if errCh != nil {
		if err := <-errCh; err != nil {
			return err
		}
	}

	// Wait for it to finish
	if ret := c.Wait(); ret != 0 {
		err := &utils.JSONError{
			Message: fmt.Sprintf("The command %v returned a non-zero code: %d", b.config.Cmd, ret),
			Code:    ret,
		}
		return err
	}

	return nil
}

// Commit the container <id> with the autorun command <autoCmd>
func (b *buildFile) commit(id string, autoCmd []string, comment string) error {
	if b.image == "" {
		return fmt.Errorf("Please provide a source image with `from` prior to commit")
	}
	b.config.Image = b.image
	if id == "" {
		cmd := b.config.Cmd
		b.config.Cmd = []string{"/bin/sh", "-c", "#(nop) " + comment}
		defer func(cmd []string) { b.config.Cmd = cmd }(cmd)

		hit, err := b.probeCache()
		if err != nil {
			return err
		}
		if hit {
			return nil
		}
		create := job.Eng.Job("create")
		create.Stdout.AddString(&id)
		job.Stderr.Add(b.outStream)
		if err := create.Env().Import(b.config); err != nil {
			return err
		}
		if err := create.Run(); err != nil {
			return err
		}
		b.tmpContainers[id] = struct{}{}
		fmt.Fprintf(b.outStream, " ---> Running in %s\n", utils.TruncateID(id))
	}
	// Note: Actually copy the struct
	autoConfig := *b.config
	autoConfig.Cmd = autoCmd
	// Commit the container
	var imageID string
	commit := job.Eng.Job("commit", id)
	commit.Setenv("author", b.maintainer)
	commit.SetenvJson("config", &autoConfig)
	commit.Stdout.AddString(&imageID)
	if err := commit.Run(); err != nil {
		return err
	}
	b.tmpImages[imageID] = struct{}{}
	b.image = imageID
	return nil
}

// Long lines can be split with a backslash
var lineContinuation = regexp.MustCompile(`\s*\\\s*\n`)

func (b *buildFile) Build(context io.Reader) (string, error) {
	tmpdirPath, err := ioutil.TempDir("", "docker-build")
	if err != nil {
		return "", err
	}

	decompressedStream, err := archive.DecompressStream(context)
	if err != nil {
		return "", err
	}

	b.context = &utils.TarSum{Reader: decompressedStream, DisableCompression: true}
	if err := archive.Untar(b.context, tmpdirPath, nil); err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpdirPath)

	b.contextPath = tmpdirPath
	filename := path.Join(tmpdirPath, "Dockerfile")
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return "", fmt.Errorf("Can't build a directory with no Dockerfile")
	}
	fileBytes, err := ioutil.ReadFile(filename)
	if err != nil {
		return "", err
	}
	if len(fileBytes) == 0 {
		return "", ErrDockerfileEmpty
	}
	var (
		dockerfile = lineContinuation.ReplaceAllString(stripComments(fileBytes), "")
		stepN      = 0
	)
	for _, line := range strings.Split(dockerfile, "\n") {
		line = strings.Trim(strings.Replace(line, "\t", " ", -1), " \t\r\n")
		if len(line) == 0 {
			continue
		}
		if err := b.BuildStep(fmt.Sprintf("%d", stepN), line); err != nil {
			return "", err
		} else if b.rm {
			b.clearTmp(b.tmpContainers)
		}
		stepN += 1
	}
	if b.image != "" {
		fmt.Fprintf(b.outStream, "Successfully built %s\n", utils.TruncateID(b.image))
		return b.image, nil
	}
	return "", fmt.Errorf("No image was generated. This may be because the Dockerfile does not, like, do anything.\n")
}

// BuildStep parses a single build step from `instruction` and executes it in the current context.
func (b *buildFile) BuildStep(name, expression string) error {
	fmt.Fprintf(b.outStream, "Step %s : %s\n", name, expression)
	tmp := strings.SplitN(expression, " ", 2)
	if len(tmp) != 2 {
		return fmt.Errorf("Invalid Dockerfile format")
	}
	instruction := strings.ToLower(strings.Trim(tmp[0], " "))
	arguments := strings.Trim(tmp[1], " ")

	method, exists := reflect.TypeOf(b).MethodByName("Cmd" + strings.ToUpper(instruction[:1]) + strings.ToLower(instruction[1:]))
	if !exists {
		fmt.Fprintf(b.errStream, "# Skipping unknown instruction %s\n", strings.ToUpper(instruction))
		return nil
	}

	ret := method.Func.Call([]reflect.Value{reflect.ValueOf(b), reflect.ValueOf(arguments)})[0].Interface()
	if ret != nil {
		return ret.(error)
	}

	fmt.Fprintf(b.outStream, " ---> %s\n", utils.TruncateID(b.image))
	return nil
}

func stripComments(raw []byte) string {
	var (
		out   []string
		lines = strings.Split(string(raw), "\n")
	)
	for _, l := range lines {
		if len(l) == 0 || l[0] == '#' {
			continue
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

func NewBuildFile(srv *Server, outStream, errStream io.Writer, verbose, utilizeCache, rm bool, outOld io.Writer, sf *utils.StreamFormatter, auth *registry.AuthConfig, authConfigFile *registry.ConfigFile) BuildFile {
	return &buildFile{
		runtime:       srv.runtime,
		srv:           srv,
		config:        &runconfig.Config{},
		outStream:     outStream,
		errStream:     errStream,
		tmpContainers: make(map[string]struct{}),
		tmpImages:     make(map[string]struct{}),
		verbose:       verbose,
		utilizeCache:  utilizeCache,
		rm:            rm,
		sf:            sf,
		authConfig:    auth,
		configFile:    authConfigFile,
		outOld:        outOld,
	}
}
