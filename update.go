package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	log "github.com/sirupsen/logrus"
	"io"
	"net/http"
	"os"
	"slices"
	"time"
)

type updateRequest struct {
	Name  string `json:"name"`
	Image string `json:"image"`
}

type updateResponse struct {
	Ok bool `json:"OK"`
}

type updateHandler struct {
	cli                *client.Client
	containerAllowList []string
}

func newUpdateHandler() *updateHandler {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatalf("can't create docker client: %v", err)
	}

	var containerAllowList []string
	if listStr, isSet := os.LookupEnv("CONTAINER_ALLOW_LIST"); isSet {
		err := json.Unmarshal([]byte(listStr), &containerAllowList)
		if err != nil {
			log.Error("Malformed CONTAINER_ALLOW_LIST")
		}
	}

	log.Debugf("CONTAINER_ALLOW_LIST: %v", containerAllowList)

	return &updateHandler{
		cli:                cli,
		containerAllowList: containerAllowList,
	}
}

func (u *updateHandler) Handler(secretKey string, timeoutInSeconds int) http.Handler {
	timeout := time.Second * time.Duration(timeoutInSeconds)

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		signature := r.Header.Get("X-Signature")
		if signature == "" || signature != secretKey {
			http.Error(w, "", http.StatusUnauthorized)
			return
		}

		if r.Method != "POST" {
			http.Error(w, "Only POST requests allowed", http.StatusBadRequest)
			return
		}

		if c := r.Header.Get("Content-Type"); c != "application/json" {
			http.Error(w, "Request body must be json", http.StatusBadRequest)
			return
		}

		var req updateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		if len(u.containerAllowList) > 0 && !slices.Contains(u.containerAllowList, req.Name) {
			log.Warnf("Container is not in allow list: %s", req.Name)
			http.Error(w, "Container is not in allow list", http.StatusBadRequest)
			return
		}

		if err := u.Update(context.Background(), &req); err != nil {
			log.Errorf("update image: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		bs, _ := json.Marshal(&updateResponse{true})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(bs)
	})

	return http.TimeoutHandler(h, timeout, fmt.Sprintf(
		"Exceeded configured timeout of %v.\n",
		timeout,
	))
}

func (u *updateHandler) Update(ctx context.Context, req *updateRequest) error {
	cli := u.cli

	oldContainerName := req.Name
	oldContainer, err := cli.ContainerInspect(context.Background(), req.Name)
	if err != nil {
		return err
	}

	// check if new image exists, if not, pull it
	_, _, err = u.cli.ImageInspectWithRaw(ctx, req.Image)
	if err != nil {
		log.Printf("Pulling image %s", req.Image)
		out, err := cli.ImagePull(ctx, req.Image, image.PullOptions{})
		if err != nil {
			return err
		}

		defer out.Close()
		_, _ = io.Copy(os.Stdout, out)
	}

	oldContainerConfigJSON, _ := json.Marshal(oldContainer.Config)
	oldContainerHostConfigJSON, _ := json.Marshal(oldContainer.HostConfig)
	var newContainerConfig container.Config
	var newHostConfig container.HostConfig

	err = json.Unmarshal(oldContainerConfigJSON, &newContainerConfig)
	if err != nil {
		return err
	}

	err = json.Unmarshal(oldContainerHostConfigJSON, &newHostConfig)
	if err != nil {
		return err
	}

	newContainerConfig.Image = req.Image

	if err := cli.ContainerStop(ctx, oldContainerName, container.StopOptions{}); err != nil {
		log.Printf("Warning: error stopping old container: %v", err)
	}

	if err := cli.ContainerRemove(ctx, oldContainerName, container.RemoveOptions{}); err != nil {
		return fmt.Errorf("error removing old container: %w", err)
	}

	resp, err := cli.ContainerCreate(ctx, &newContainerConfig, &newHostConfig, nil, nil, oldContainerName)
	if err != nil {
		return fmt.Errorf("error creating new container: %w", err)
	}
	newContainerID := resp.ID

	// re-connect to networks
	for networkName, _ := range oldContainer.NetworkSettings.Networks {
		err := cli.NetworkConnect(ctx, networkName, newContainerID, &network.EndpointSettings{})
		if err != nil {
			return err
		}
	}

	if err := cli.ContainerStart(ctx, newContainerID, container.StartOptions{}); err != nil {
		// If start fails, try to remove the created container to cleanup
		removeOptions := container.RemoveOptions{Force: true}
		_ = cli.ContainerRemove(ctx, newContainerID, removeOptions)
		return fmt.Errorf("error starting new container: %w (cleanup attempted)", err)
	}

	return nil
}
