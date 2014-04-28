package registry

import (
	"fmt"
	"github.com/dotcloud/docker/engine"
	"github.com/dotcloud/docker/utils"
	"sync"
)

// Service exposes registry capabilities in the standard Engine
// interface. Once installed, it extends the engine with the
// following calls:
//
//  'auth': Authenticate against the public registry
//  'search': Search for images on the public registry
//  'pull': Download images from any registry
//  'push': Upload images to any registry (TODO)
type Service struct {
	sync.RWMutex
	pullingPool map[string]chan struct{}
	pushingPool map[string]chan struct{}
}

// NewService returns a new instance of Service ready to be
// installed no an engine.
func NewService() *Service {
	return &Service{
		pullingPool: make(map[string]chan struct{}),
		pushingPool: make(map[string]chan struct{}),
	}
}

// Install installs registry capabilities to eng.
func (s *Service) Install(eng *engine.Engine) error {
	eng.Register("auth", s.Auth)
	eng.Register("search", s.Search)
	eng.Register("pull", s.Pull)
	eng.Register("push", s.Pull)
	return nil
}

// Auth contacts the public registry with the provided credentials,
// and returns OK if authentication was sucessful.
// It can be used to verify the validity of a client's credentials.
func (s *Service) Auth(job *engine.Job) engine.Status {
	var (
		err        error
		authConfig = &AuthConfig{}
	)

	job.GetenvJson("authConfig", authConfig)
	// TODO: this is only done here because auth and registry need to be merged into one pkg
	if addr := authConfig.ServerAddress; addr != "" && addr != IndexServerAddress() {
		addr, err = ExpandAndVerifyRegistryUrl(addr)
		if err != nil {
			return job.Error(err)
		}
		authConfig.ServerAddress = addr
	}
	status, err := Login(authConfig, HTTPRequestFactory(nil))
	if err != nil {
		return job.Error(err)
	}
	job.Printf("%s\n", status)
	return engine.StatusOK
}

// Search queries the public registry for images matching the specified
// search terms, and returns the results.
//
// Argument syntax: search TERM
//
// Option environment:
//	'authConfig': json-encoded credentials to authenticate against the registry.
//		The search extends to images only accessible via the credentials.
//
//	'metaHeaders': extra HTTP headers to include in the request to the registry.
//		The headers should be passed as a json-encoded dictionary.
//
// Output:
//	Results are sent as a collection of structured messages (using engine.Table).
//	Each result is sent as a separate message.
//	Results are ordered by number of stars on the public registry.
func (s *Service) Search(job *engine.Job) engine.Status {
	if n := len(job.Args); n != 1 {
		return job.Errorf("Usage: %s TERM", job.Name)
	}
	var (
		term        = job.Args[0]
		metaHeaders = map[string][]string{}
		authConfig  = &AuthConfig{}
	)
	job.GetenvJson("authConfig", authConfig)
	job.GetenvJson("metaHeaders", metaHeaders)

	r, err := NewRegistry(authConfig, HTTPRequestFactory(metaHeaders), IndexServerAddress())
	if err != nil {
		return job.Error(err)
	}
	results, err := r.SearchRepositories(term)
	if err != nil {
		return job.Error(err)
	}
	outs := engine.NewTable("star_count", 0)
	for _, result := range results.Results {
		out := &engine.Env{}
		out.Import(result)
		outs.Add(out)
	}
	outs.ReverseSort()
	if _, err := outs.WriteListTo(job.Stdout); err != nil {
		return job.Error(err)
	}
	return engine.StatusOK
}

func (s *Service) Pull(job *engine.Job) engine.Status {
	if n := len(job.Args); n != 1 && n != 2 {
		return job.Errorf("Usage: %s IMAGE [TAG]", job.Name)
	}
	var (
		localName   = job.Args[0]
		tag         string
		sf          = utils.NewStreamFormatter(job.GetenvBool("json"))
		authConfig  AuthConfig
		configFile  = &ConfigFile{}
		metaHeaders map[string][]string
	)
	if len(job.Args) > 1 {
		tag = job.Args[1]
	}

	job.GetenvJson("auth", configFile)
	job.GetenvJson("metaHeaders", metaHeaders)

	endpoint, _, err := ResolveRepositoryName(localName)
	if err != nil {
		return job.Error(err)
	}
	authConfig = configFile.ResolveAuthConfig(endpoint)

	c, err := s.poolAdd("pull", localName+":"+tag)
	if err != nil {
		if c != nil {
			// Another pull of the same repository is already taking place; just wait for it to finish
			job.Stdout.Write(sf.FormatStatus("", "Repository %s already being pulled by another client. Waiting.", localName))
			<-c
			return engine.StatusOK
		}
		return job.Error(err)
	}
	defer s.poolRemove("pull", localName+":"+tag)

	// Resolve the Repository name from fqn to endpoint + name
	hostname, remoteName, err := ResolveRepositoryName(localName)
	if err != nil {
		return job.Error(err)
	}

	endpoint, err = ExpandAndVerifyRegistryUrl(hostname)
	if err != nil {
		return job.Error(err)
	}

	r, err := NewRegistry(&authConfig, HTTPRequestFactory(metaHeaders), endpoint)
	if err != nil {
		return job.Error(err)
	}

	if endpoint == IndexServerAddress() {
		// If pull "index.docker.io/foo/bar", it's stored locally under "foo/bar"
		localName = remoteName
	}

	if err = s.pullRepository(job.Eng, r, job.Stdout, localName, remoteName, tag, sf, job.GetenvBool("parallel")); err != nil {
		return job.Error(err)
	}

	return engine.StatusOK
}


// FIXME: Allow to interrupt current push when new push of same image is done.
func (s *Service) Push(job *engine.Job) engine.Status {
	if n := len(job.Args); n != 1 {
		return job.Errorf("Usage: %s IMAGE", job.Name)
	}
	var (
		localName   = job.Args[0]
		sf          = utils.NewStreamFormatter(job.GetenvBool("json"))
		authConfig  = &registry.AuthConfig{}
		metaHeaders map[string][]string
	)

	tag := job.Getenv("tag")
	job.GetenvJson("authConfig", authConfig)
	job.GetenvJson("metaHeaders", metaHeaders)
	if _, err := s.poolAdd("push", localName); err != nil {
		return job.Error(err)
	}
	defer s.poolRemove("push", localName)

	// Resolve the Repository name from fqn to endpoint + name
	hostname, remoteName, err := registry.ResolveRepositoryName(localName)
	if err != nil {
		return job.Error(err)
	}

	endpoint, err := registry.ExpandAndVerifyRegistryUrl(hostname)
	if err != nil {
		return job.Error(err)
	}

	img, err := srv.daemon.Graph().Get(localName)
	r, err2 := registry.NewRegistry(authConfig, registry.HTTPRequestFactory(metaHeaders), endpoint)
	if err2 != nil {
		return job.Error(err2)
	}

	if err != nil {
		reposLen := 1
		if tag == "" {
			reposLen = len(srv.daemon.Repositories().Repositories[localName])
		}
		job.Stdout.Write(sf.FormatStatus("", "The push refers to a repository [%s] (len: %d)", localName, reposLen))
		// If it fails, try to get the repository
		if localRepo, exists := srv.daemon.Repositories().Repositories[localName]; exists {
			if err := s.pushRepository(job.Eng, r, job.Stdout, localName, remoteName, localRepo, tag, sf); err != nil {
				return job.Error(err)
			}
			return engine.StatusOK
		}
		return job.Error(err)
	}

	var token []string
	job.Stdout.Write(sf.FormatStatus("", "The push refers to an image: [%s]", localName))
	if _, err := s.pushImage(job.Eng, r, job.Stdout, remoteName, img.ID, endpoint, token, sf); err != nil {
		return job.Error(err)
	}
	return engine.StatusOK
}




func (s *Service) poolAdd(kind, key string) (chan struct{}, error) {
	s.Lock()
	defer s.Unlock()

	if c, exists := s.pullingPool[key]; exists {
		return c, fmt.Errorf("pull %s is already in progress", key)
	}
	if c, exists := s.pushingPool[key]; exists {
		return c, fmt.Errorf("push %s is already in progress", key)
	}

	c := make(chan struct{})
	switch kind {
	case "pull":
		s.pullingPool[key] = c
	case "push":
		s.pushingPool[key] = c
	default:
		return nil, fmt.Errorf("Unknown pool type")
	}
	return c, nil
}

func (s *Service) poolRemove(kind, key string) error {
	s.Lock()
	defer s.Unlock()
	switch kind {
	case "pull":
		if c, exists := s.pullingPool[key]; exists {
			close(c)
			delete(s.pullingPool, key)
		}
	case "push":
		if c, exists := s.pushingPool[key]; exists {
			close(c)
			delete(s.pushingPool, key)
		}
	default:
		return fmt.Errorf("Unknown pool type")
	}
	return nil
}
