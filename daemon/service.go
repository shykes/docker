package daemon

import (
	"github.com/dotcloud/docker/engine"
)

func (d *Daemon) Install(eng *engine.Engine) error {

}

func (d *Daemon) CmdDestroy(job *engine.Job) engine.Status {
	if len(job.Args) != 1 {
		return job.Errorf("Not enough arguments. Usage: %s CONTAINER\n", job.Name)
	}
	name := job.Args[0]
	removeVolume := job.GetenvBool("removeVolume")
	removeLink := job.GetenvBool("removeLink")
	forceRemove := job.GetenvBool("forceRemove")

	container := d.Get(name)

	if removeLink {
		if container == nil {
			return job.Errorf("No such link: %s", name)
		}
		name, err := daemon.GetFullContainerName(name)
		if err != nil {
			job.Error(err)
		}
		parent, n := path.Split(name)
		if parent == "/" {
			return job.Errorf("Conflict, cannot remove the default name of the container")
		}
		pe := d.ContainerGraph().Get(parent)
		if pe == nil {
			return job.Errorf("Cannot get parent %s for name %s", parent, name)
		}
		parentContainer := d.Get(pe.ID())

		if parentContainer != nil {
			parentContainer.DisableLink(n)
		}

		if err := d.ContainerGraph().Delete(name); err != nil {
			return job.Error(err)
		}
		return engine.StatusOK
	}

	if container != nil {
		if container.State.IsRunning() {
			if forceRemove {
				if err := container.Stop(5); err != nil {
					return job.Errorf("Could not stop running container, cannot remove - %v", err)
				}
			} else {
				return job.Errorf("Impossible to remove a running container, please stop it first or use -f")
			}
		}
		if err := d.Destroy(container); err != nil {
			return job.Errorf("Cannot destroy container %s: %s", name, err)
		}
		// YOU ARE HERE: how do we get rid of this dependency on srv?
		// We might need to move LogEvent off to a separate job before anythign else... great...
		srv.LogEvent("destroy", container.ID, d.Repositories().ImageName(container.Image))

		if removeVolume {
			var (
				volumes     = make(map[string]struct{})
				binds       = make(map[string]struct{})
				usedVolumes = make(map[string]*daemon.Container)
			)

			// the volume id is always the base of the path
			getVolumeId := func(p string) string {
				return filepath.Base(strings.TrimSuffix(p, "/layer"))
			}

			// populate bind map so that they can be skipped and not removed
			for _, bind := range container.HostConfig().Binds {
				source := strings.Split(bind, ":")[0]
				// TODO: refactor all volume stuff, all of it
				// it is very important that we eval the link or comparing the keys to container.Volumes will not work
				//
				// eval symlink can fail, ref #5244 if we receive an is not exist error we can ignore it
				p, err := filepath.EvalSymlinks(source)
				if err != nil && !os.IsNotExist(err) {
					return job.Error(err)
				}
				if p != "" {
					source = p
				}
				binds[source] = struct{}{}
			}

			// Store all the deleted containers volumes
			for _, volumeId := range container.Volumes {
				// Skip the volumes mounted from external
				// bind mounts here will will be evaluated for a symlink
				if _, exists := binds[volumeId]; exists {
					continue
				}

				volumeId = getVolumeId(volumeId)
				volumes[volumeId] = struct{}{}
			}

			// Retrieve all volumes from all remaining containers
			for _, container := range d.List() {
				for _, containerVolumeId := range container.Volumes {
					containerVolumeId = getVolumeId(containerVolumeId)
					usedVolumes[containerVolumeId] = container
				}
			}

			for volumeId := range volumes {
				// If the requested volu
				if c, exists := usedVolumes[volumeId]; exists {
					log.Printf("The volume %s is used by the container %s. Impossible to remove it. Skipping.\n", volumeId, c.ID)
					continue
				}
				if err := d.Volumes().Delete(volumeId); err != nil {
					return job.Errorf("Error calling volumes.Delete(%q): %v", volumeId, err)
				}
			}
		}
	} else {
		return job.Errorf("No such container: %s", name)
	}
	return engine.StatusOK
}

