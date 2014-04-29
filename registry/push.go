package registry

import (
)

func (s *Service) pushRepository(r *Registry, out io.Writer, localName, remoteName string, localRepo map[string]string, tag string, sf *utils.StreamFormatter) error {
	out = utils.NewWriteFlusher(out)
	utils.Debugf("Local repo: %s", localRepo)
	imgList, tagsByImage, err := s.getImageList(localRepo, tag)
	if err != nil {
		return err
	}

	out.Write(sf.FormatStatus("", "Sending image list"))

	var (
		repoData   *registry.RepositoryData
		imageIndex []*registry.ImgData
	)

	for _, imgId := range imgList {
		if tags, exists := tagsByImage[imgId]; exists {
			// If an image has tags you must add an entry in the image index
			// for each tag
			for _, tag := range tags {
				imageIndex = append(imageIndex, &registry.ImgData{
					ID:  imgId,
					Tag: tag,
				})
			}
		} else {
			// If the image does not have a tag it still needs to be sent to the
			// registry with an empty tag so that it is accociated with the repository
			imageIndex = append(imageIndex, &registry.ImgData{
				ID:  imgId,
				Tag: "",
			})

		}
	}

	utils.Debugf("Preparing to push %s with the following images and tags\n", localRepo)
	for _, data := range imageIndex {
		utils.Debugf("Pushing ID: %s with Tag: %s\n", data.ID, data.Tag)
	}

	// Register all the images in a repository with the registry
	// If an image is not in this list it will not be associated with the repository
	repoData, err = r.PushImageJSONIndex(remoteName, imageIndex, false, nil)
	if err != nil {
		return err
	}

	nTag := 1
	if tag == "" {
		nTag = len(localRepo)
	}
	for _, ep := range repoData.Endpoints {
		out.Write(sf.FormatStatus("", "Pushing repository %s (%d tags)", localName, nTag))

		for _, imgId := range imgList {
			if r.LookupRemoteImage(imgId, ep, repoData.Tokens) {
				out.Write(sf.FormatStatus("", "Image %s already pushed, skipping", utils.TruncateID(imgId)))
			} else {
				if _, err := s.pushImage(r, out, remoteName, imgId, ep, repoData.Tokens, sf); err != nil {
					// FIXME: Continue on error?
					return err
				}
			}

			for _, tag := range tagsByImage[imgId] {
				out.Write(sf.FormatStatus("", "Pushing tag for rev [%s] on {%s}", utils.TruncateID(imgId), ep+"repositories/"+remoteName+"/tags/"+tag))

				if err := r.PushRegistryTag(remoteName, imgId, tag, ep, repoData.Tokens); err != nil {
					return err
				}
			}
		}
	}

	if _, err := r.PushImageJSONIndex(remoteName, imageIndex, true, repoData.Endpoints); err != nil {
		return err
	}

	return nil
}

func (s *Service) pushImage(eng *engine.Engine, r *Registry, out io.Writer, remote, imgID, ep string, token []string, sf *utils.StreamFormatter) (checksum string, err error) {
	out = utils.NewWriteFlusher(out)
	img, err := eng.Job("image_get", imgID)
	err != nil {
		return "", err
	}
	jsonRaw := img.Get("json")
	if jsonRaw == "" {
		return "", fmt.Errorf("cannot retrieve image raw json")
	}
	out.Write(sf.FormatProgress(utils.TruncateID(imgID), "Pushing", nil))

	imgData := &registry.ImgData{
		ID: imgID,
	}

	// Send the json
	if err := r.PushImageJSONRegistry(imgData, jsonRaw, ep, token); err != nil {
		if err == registry.ErrAlreadyExists {
			out.Write(sf.FormatProgress(utils.TruncateID(imgData.ID), "Image already pushed, skipping", nil))
			return "", nil
		}
		return "", err
	}


	getLayer := eng.Job("image_get_layer", imgID)
	layerData, err := getLayer.Stdout.AddPipe()
	if err != nil {
		return job.Error(err)
	}
	getLayer.Stderr.Add(out)
	layerData, err := srv.daemon.Graph().TempLayerArchive(imgID, archive.Uncompressed, sf, out)
	if err != nil {
		return "", fmt.Errorf("Failed to generate layer archive: %s", err)
	}
	defer os.RemoveAll(layerData.Name())

	// Send the layer
	utils.Debugf("rendered layer for %s of [%d] size", imgData.ID, layerData.Size)

	checksum, checksumPayload, err := r.PushImageLayerRegistry(imgData.ID, utils.ProgressReader(layerData, int(layerData.Size), out, sf, false, utils.TruncateID(imgData.ID), "Pushing"), ep, token, jsonRaw)
	if err != nil {
		return "", err
	}
	imgData.Checksum = checksum
	imgData.ChecksumPayload = checksumPayload
	// Send the checksum
	if err := r.PushImageChecksumRegistry(imgData, ep, token); err != nil {
		return "", err
	}

	out.Write(sf.FormatProgress(utils.TruncateID(imgData.ID), "Image successfully pushed", nil))
	return imgData.Checksum, nil
}

// Retrieve the all the images to be uploaded in the correct order
func (s *Service) getImageList(eng *engine.Engine, localRepo map[string]string, requestedTag string) ([]string, map[string][]string, error) {
	var (
		imageList   []string
		imagesSeen  map[string]bool     = make(map[string]bool)
		tagsByImage map[string][]string = make(map[string][]string)
	)

	for tag, id := range localRepo {
		if requestedTag != "" && requestedTag != tag {
			continue
		}
		var imageListForThisTag []string

		tagsByImage[id] = append(tagsByImage[id], tag)

		for {
			img, err := eng.Job("image_get", id).RunReceive()
			if err != nil {
				return nil, nil, err
			}
			if img.Len() == 0 {
				break
			}
			if imagesSeen[img.ID] {
				// This image is already on the list, we can ignore it and all its parents
				break
			}
			imagesSeen[img.ID] = true
			imageListForThisTag = append(imageListForThisTag, img.ID)
			if parentID := img.Get("parent_id"); parentID == "" {
				break
			} else {
				img, err = eng.Job("image_get", parentID)
				if err != nil {
					return err
				}
				if img.Len() == 0 {
					break
				}
			}
		}

		// reverse the image list for this tag (so the "most"-parent image is first)
		for i, j := 0, len(imageListForThisTag)-1; i < j; i, j = i+1, j-1 {
			imageListForThisTag[i], imageListForThisTag[j] = imageListForThisTag[j], imageListForThisTag[i]
		}

		// append to main image list
		imageList = append(imageList, imageListForThisTag...)
	}
	if len(imageList) == 0 {
		return nil, nil, fmt.Errorf("No images found for the requested repository / tag")
	}
	utils.Debugf("Image list: %v", imageList)
	utils.Debugf("Tags by image: %v", tagsByImage)

	return imageList, tagsByImage, nil
}
