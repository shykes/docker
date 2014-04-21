package graph

import (
	"fmt"
	"github.com/dotcloud/docker/engine"
	"github.com/dotcloud/docker/registry"
	"github.com/dotcloud/docker/runconfig"
	"github.com/dotcloud/docker/utils"
	"github.com/dotcloud/docker/image"
)

func (store *TagStore) Install(eng *engine.Engine) error {
	eng.Register("tag", func(job *engine.Job) engine.Status {
		if len(job.Args) != 2 && len(job.Args) != 3 {
			return job.Errorf("Usage: %s IMAGE REPOSITORY [TAG]\n", job.Name)
		}
		var tag string
		if len(job.Args) == 3 {
			tag = job.Args[2]
		}
		if err := store.Set(job.Args[1], tag, job.Args[0], job.GetenvBool("force")); err != nil {
			return job.Error(err)
		}
		return engine.StatusOK
	})
	// FIXME: deprecate this in favor of a more generic 'image_search'
	eng.Register("image_byparent", func(job *engine.Job) engine.Status {
		if len(job.Args) != 2 {
			return job.Errorf("usage: %s PARENT", job.Name)
		}
		imgID := job.Args[0]
		config := &runconfig.Config{}
		job.GetenvJson("config", config)
		// Retrieve all images
		images, err := store.graph.Map()
		if err != nil {
			return job.Errorf("%v\n", err)
		}

		// Store the tree in a map of map (map[parentId][childId])
		imageMap := make(map[string]map[string]struct{})
		for _, img := range images {
			if _, exists := imageMap[img.Parent]; !exists {
				imageMap[img.Parent] = make(map[string]struct{})
			}
			imageMap[img.Parent][img.ID] = struct{}{}
		}

		// Loop on the children of the given image and check the config
		var match *image.Image
		for elem := range imageMap[imgID] {
			img, err := store.graph.Get(elem)
			if err != nil {
				return job.Errorf("%v\n", err)
			}
			if runconfig.Compare(&img.ContainerConfig, config) {
				if match == nil || match.Created.Before(img.Created) {
					match = img
				}
			}
		}
		fmt.Fprintf(job.Stdout, "%s\n", match.ID)
		return engine.StatusOK
	})
	eng.Register("getimage", func(job *engine.Job) engine.Status {
		if len(job.Args) != 1 {
			return job.Errorf("usage: %s NAME", job.Name)
		}
		// Read arguments
		name := job.Args[0]
		autopull := job.GetenvBool("autopull")
		auth := &registry.ConfigFile{}
		job.GetenvJson("auth", auth)

		// 
		image, err := store.LookupImage(name)
		if err != nil {
			if !autopull {
				return job.Errorf("%v\n", err)
			}
			if IsNotExist(err) {
				remote, tag := utils.ParseRepositoryTag(name)
				pull := job.Eng.Job("pull", remote, tag)
				pull.Setenv("json", job.Getenv("json"))
				pull.SetenvBool("parallel", true)

				// Convert auth to the deprecated 'authConfig' format still
				// required by 'pull'.
				//
				// FIXME: fix 'pull' to consume the standard auth object
				// (of type registry.ConfigFile, and by the way that's a terrible
				// type name let's please change it). -- Solomon
				if len(auth.Configs) > 0 {
					// The request came with a full auth config file, we prefer to use that
					endpoint, _, err := registry.ResolveRepositoryName(remote)
					if err != nil {
						return job.Errorf("%v\n", err)
					}
					pullAuth := auth.ResolveAuthConfig(endpoint)
					pull.SetenvJson("authConfig", &pullAuth)
				}
				// Pull output goes to stdout, to avoid conflict with the result.
				pull.Stdout.Add(job.Stderr)
				if err := job.Run(); err != nil {
					return job.Errorf("%v\n", err)
				}
				image, err = store.LookupImage(name)
				if err != nil {
					return job.Errorf("%v\n", err)
				}
			} else {
				return job.Errorf("%v\n", err)
			}
		}
		i := &engine.Env{}
		i.Set("id", image.ID)
		i.SetJson("config", image.Config)
		i.WriteTo(job.Stdout)
		return engine.StatusOK
	})
	return nil
}
