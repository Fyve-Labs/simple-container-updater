package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/docker/cli/cli/config"
	"github.com/docker/docker/api/types/image"
	log "github.com/sirupsen/logrus"
	"io"
	"os"
	"strings"
)

func (u *updateHandler) PullImage(ctx context.Context, imageUrl string) error {
	// check if new image exists, if not, pull it
	_, _, err := u.cli.ImageInspectWithRaw(ctx, imageUrl)
	if err != nil {
		log.Printf("Pulling image %s", imageUrl)
		options := image.PullOptions{}

		configfile, err := config.Load(config.Dir())
		if err != nil {
			log.Errorf("Read config file: %v", err)
		} else {
			slashIndex := strings.Index(imageUrl, "/")
			if slashIndex >= 1 && strings.ContainsAny(imageUrl[:slashIndex], ".:") {
				repoURL := strings.Split(imageUrl, "/")[0]
				creds, err := configfile.GetCredentialsStore(repoURL).Get(repoURL)

				if err != nil {
					log.Errorf("DockerPull: %v", err)
				} else {
					encodedJSON, _ := json.Marshal(creds)
					options.RegistryAuth = base64.URLEncoding.EncodeToString(encodedJSON)
				}
			}
		}

		out, err := u.cli.ImagePull(ctx, imageUrl, options)
		if err != nil {
			return err
		}

		defer func(out io.ReadCloser) {
			_ = out.Close()
		}(out)

		_, err = io.Copy(os.Stdout, out)

		return err
	}

	return nil
}
