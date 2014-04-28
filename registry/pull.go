package registry

import (
	"fmt"
	"github.com/dotcloud/docker/engine"
	"github.com/dotcloud/docker/image"
	"github.com/dotcloud/docker/utils"
	"io"
	"time"
)

func (s *Service) pullRepository(eng *engine.Engine, r *Registry, out io.Writer, localName, remoteName, askedTag string, sf *utils.StreamFormatter, parallel bool) error {
	out.Write(sf.FormatStatus("", "Pulling repository %s", localName))

	repoData, err := r.GetRepositoryData(remoteName)
	if err != nil {
		return err
	}

	utils.Debugf("Retrieving the tag list")
	tagsList, err := r.GetRemoteTags(repoData.Endpoints, remoteName, repoData.Tokens)
	if err != nil {
		utils.Errorf("%v", err)
		return err
	}

	for tag, id := range tagsList {
		repoData.ImgList[id] = &ImgData{
			ID:       id,
			Tag:      tag,
			Checksum: "",
		}
	}

	utils.Debugf("Registering tags")
	// If no tag has been specified, pull them all
	if askedTag == "" {
		for tag, id := range tagsList {
			repoData.ImgList[id].Tag = tag
		}
	} else {
		// Otherwise, check that the tag exists and use only that one
		id, exists := tagsList[askedTag]
		if !exists {
			return fmt.Errorf("Tag %s not found in repository %s", askedTag, localName)
		}
		repoData.ImgList[id].Tag = askedTag
	}

	errors := make(chan error)
	for _, image := range repoData.ImgList {
		downloadImage := func(img *ImgData) {
			if askedTag != "" && img.Tag != askedTag {
				utils.Debugf("(%s) does not match %s (id: %s), skipping", img.Tag, askedTag, img.ID)
				if parallel {
					errors <- nil
				}
				return
			}

			if img.Tag == "" {
				utils.Debugf("Image (id: %s) present in this repository but untagged, skipping", img.ID)
				if parallel {
					errors <- nil
				}
				return
			}

			// ensure no two downloads of the same image happen at the same time
			if c, err := s.poolAdd("pull", "img:"+img.ID); err != nil {
				if c != nil {
					out.Write(sf.FormatProgress(utils.TruncateID(img.ID), "Layer already being pulled by another client. Waiting.", nil))
					<-c
					out.Write(sf.FormatProgress(utils.TruncateID(img.ID), "Download complete", nil))
				} else {
					utils.Errorf("Image (id: %s) pull is already running, skipping: %v", img.ID, err)
				}
				if parallel {
					errors <- nil
				}
				return
			}
			defer s.poolRemove("pull", "img:"+img.ID)

			out.Write(sf.FormatProgress(utils.TruncateID(img.ID), fmt.Sprintf("Pulling image (%s) from %s", img.Tag, localName), nil))
			success := false
			var lastErr error
			for _, ep := range repoData.Endpoints {
				out.Write(sf.FormatProgress(utils.TruncateID(img.ID), fmt.Sprintf("Pulling image (%s) from %s, endpoint: %s", img.Tag, localName, ep), nil))
				if err := s.pullImage(eng, r, out, img.ID, ep, repoData.Tokens, sf); err != nil {
					// It's not ideal that only the last error is returned, it would be better to concatenate the errors.
					// As the error is also given to the output stream the user will see the error.
					lastErr = err
					out.Write(sf.FormatProgress(utils.TruncateID(img.ID), fmt.Sprintf("Error pulling image (%s) from %s, endpoint: %s, %s", img.Tag, localName, ep, err), nil))
					continue
				}
				success = true
				break
			}
			if !success {
				out.Write(sf.FormatProgress(utils.TruncateID(img.ID), fmt.Sprintf("Error pulling image (%s) from %s, %s", img.Tag, localName, lastErr), nil))
				if parallel {
					errors <- fmt.Errorf("Could not find repository on any of the indexed registries.")
					return
				}
			}
			out.Write(sf.FormatProgress(utils.TruncateID(img.ID), "Download complete", nil))

			if parallel {
				errors <- nil
			}
		}

		if parallel {
			go downloadImage(image)
		} else {
			downloadImage(image)
		}
	}
	if parallel {
		var lastError error
		for i := 0; i < len(repoData.ImgList); i++ {
			if err := <-errors; err != nil {
				lastError = err
			}
		}
		if lastError != nil {
			return lastError
		}

	}
	for tag, id := range tagsList {
		if askedTag != "" && tag != askedTag {
			continue
		}
		if err := eng.Job("image_tag", localName+":"+tag, id).Run(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) pullImage(eng *engine.Engine, r *Registry, out io.Writer, imgID, endpoint string, token []string, sf *utils.StreamFormatter) error {
	history, err := r.GetRemoteHistory(imgID, endpoint, token)
	if err != nil {
		return err
	}
	out.Write(sf.FormatProgress(utils.TruncateID(imgID), "Pulling dependent layers", nil))
	// FIXME: Try to stream the images?
	// FIXME: Launch the getRemoteImage() in goroutines

	for i := len(history) - 1; i >= 0; i-- {
		id := history[i]

		// ensure no two downloads of the same layer happen at the same time
		if c, err := s.poolAdd("pull", "layer:"+id); err != nil {
			utils.Errorf("Image (id: %s) pull is already running, skipping: %v", id, err)
			<-c
		}
		defer s.poolRemove("pull", "layer:"+id)

		// Don't download the layer if the image already exists.
		// Note: this is racy, but it's OK because it's just an optimization.
		// Worst case we get a clean error after we started streaming the layer
		// from the registry.
		if img, err := eng.Job("image_get", id).RunReceive(); err != nil {
			return err
		} else if img.Len() == 0 { // empty entry means the image doesn't exist.
			out.Write(sf.FormatProgress(utils.TruncateID(id), "Pulling metadata", nil))
			var (
				imgJSON []byte
				imgSize int
				err     error
				img     *image.Image
			)
			retries := 5
			for j := 1; j <= retries; j++ {
				imgJSON, imgSize, err = r.GetRemoteImageJSON(id, endpoint, token)
				if err != nil && j == retries {
					out.Write(sf.FormatProgress(utils.TruncateID(id), "Error pulling dependent layers", nil))
					return err
				} else if err != nil {
					time.Sleep(time.Duration(j) * 500 * time.Millisecond)
					continue
				}
				img, err = image.NewImgJSON(imgJSON)
				if err != nil && j == retries {
					out.Write(sf.FormatProgress(utils.TruncateID(id), "Error pulling dependent layers", nil))
					return fmt.Errorf("Failed to parse json: %s", err)
				} else if err != nil {
					time.Sleep(time.Duration(j) * 500 * time.Millisecond)
					continue
				} else {
					break
				}
			}

			// Get the layer
			out.Write(sf.FormatProgress(utils.TruncateID(id), "Pulling fs layer", nil))
			layer, err := r.GetRemoteImageLayer(img.ID, endpoint, token)
			if err != nil {
				out.Write(sf.FormatProgress(utils.TruncateID(id), "Error pulling dependent layers", nil))
				return err
			}
			defer layer.Close()
			// Store the image in the graph
			imgSet := eng.Job("image_set", id)
			// Wrap the layer archive in a progress bar generator
			// FIXME: 'graph_set' should take care of printing a progress bar on stderr.
			//	(note: this will require passing a size hint to graph_set)
			layerProgress := utils.ProgressReader(layer, imgSize, out, sf, false, utils.TruncateID(id), "Downloading")
			imgSet.Stdin.Add(layerProgress)
			imgSet.Setenv("json", string(imgJSON))
			if err := imgSet.Run(); err != nil {
				out.Write(sf.FormatProgress(utils.TruncateID(id), "Error downloading dependent layers", nil))
				return err
			}
		}
		out.Write(sf.FormatProgress(utils.TruncateID(id), "Download complete", nil))

	}
	return nil
}
