package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	log "github.com/sirupsen/logrus"
)

func (u *updateHandler) Update(ctx context.Context, req *updateRequest) error {
	cli := u.cli

	oldContainerName := req.Name
	oldContainer, err := cli.ContainerInspect(context.Background(), req.Name)
	if err != nil {
		return err
	}

	if err = u.PullImage(ctx, req.Image); err != nil {
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
