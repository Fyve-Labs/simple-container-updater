package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	log "github.com/sirupsen/logrus"
	"os"
	"slices"
)

type UpdaterInterface interface {
	Update(ctx context.Context, containerId, newImage string) error
}

type updateHandler struct {
	cli                *client.Client
	ContainerAllowList []string
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
		ContainerAllowList: containerAllowList,
	}
}

func (u *updateHandler) Update(ctx context.Context, containerId, newImage string) error {
	if len(u.ContainerAllowList) > 0 && !slices.Contains(u.ContainerAllowList, containerId) {
		return fmt.Errorf("container is not in allow list: %s", containerId)
	}

	cli := u.cli

	oldContainerName := containerId
	oldContainer, err := cli.ContainerInspect(context.Background(), containerId)
	if err != nil {
		return err
	}

	if err = u.PullImage(ctx, newImage); err != nil {
		return err
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

	newContainerConfig.Image = newImage

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
