/*
2020 © Postgres.ai
*/

// Package cont provides tools to manage service containers started by Database Lab Engine.
package cont

import (
	"context"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/docker/go-units"
	"github.com/pkg/errors"

	"gitlab.com/postgres-ai/database-lab/v2/pkg/log"
	"gitlab.com/postgres-ai/database-lab/v2/pkg/retrieval/engine/postgres/tools"
	"gitlab.com/postgres-ai/database-lab/v2/pkg/retrieval/options"
)

const (
	labelFilter = "label"

	// StopTimeout defines a container stop timeout.
	StopTimeout = 30 * time.Second

	// StopPhysicalTimeout defines stop timeout for a physical container.
	StopPhysicalTimeout = 5 * time.Second

	// SyncInstanceContainerPrefix defines a sync container name.
	SyncInstanceContainerPrefix = "dblab_sync_"

	// DBLabControlLabel defines a label to mark service containers.
	DBLabControlLabel = "dblab_control"
	// DBLabInstanceIDLabel defines a label to mark service containers related to the current Database Lab instance.
	DBLabInstanceIDLabel = "dblab_instance_id"

	// DBLabSyncLabel defines a label value for sync containers.
	DBLabSyncLabel = "dblab_sync"
	// DBLabPromoteLabel defines a label value for promote containers.
	DBLabPromoteLabel = "dblab_promote"
	// DBLabPatchLabel defines a label value for patch containers.
	DBLabPatchLabel = "dblab_patch"
	// DBLabDumpLabel defines a label value for dump containers.
	DBLabDumpLabel = "dblab_dump"
	// DBLabRestoreLabel defines a label value for restore containers.
	DBLabRestoreLabel = "dblab_restore"

	// DBLabRunner defines a label to mark runner containers.
	DBLabRunner = "dblab_runner"
)

// TODO(akartasov): Control container manager.

// StopControlContainers stops control containers run by Database Lab Engine.
func StopControlContainers(ctx context.Context, dockerClient *client.Client, instanceID, dataDir string) error {
	log.Msg("Stop control containers")

	list, err := getControlContainerList(ctx, dockerClient, instanceID)
	if err != nil {
		return err
	}

	for _, controlCont := range list {
		containerName := getControlContainerName(controlCont)

		controlLabel, ok := controlCont.Labels[DBLabControlLabel]
		if !ok {
			log.Msg("Control label not found for container: ", containerName)
			continue
		}

		if shouldStopInternalProcess(controlLabel) {
			log.Msg("Stopping control container: ", containerName)

			if err := tools.StopPostgres(ctx, dockerClient, controlCont.ID, dataDir, tools.DefaultStopTimeout); err != nil {
				log.Msg("Failed to stop Postgres", err)
				tools.PrintContainerLogs(ctx, dockerClient, controlCont.ID)

				continue
			}
		}

		log.Msg("Removing control container:", containerName)

		if err := dockerClient.ContainerRemove(ctx, controlCont.ID, types.ContainerRemoveOptions{
			RemoveVolumes: true,
			Force:         true,
		}); err != nil {
			return err
		}
	}

	return nil
}

// CleanUpControlContainers removes control containers run by Database Lab Engine.
func CleanUpControlContainers(ctx context.Context, dockerClient *client.Client, instanceID string) error {
	log.Msg("Cleanup control containers")

	list, err := getControlContainerList(ctx, dockerClient, instanceID)
	if err != nil {
		return err
	}

	for _, controlCont := range list {
		log.Msg("Removing control container:", getControlContainerName(controlCont))

		if err := dockerClient.ContainerRemove(ctx, controlCont.ID, types.ContainerRemoveOptions{
			RemoveVolumes: true,
			Force:         true,
		}); err != nil {
			return err
		}
	}

	return nil
}

func getControlContainerList(ctx context.Context, dockerClient *client.Client, instanceID string) ([]types.Container, error) {
	filterPairs := []filters.KeyValuePair{
		{
			Key:   labelFilter,
			Value: DBLabControlLabel,
		},
		{
			Key:   labelFilter,
			Value: DBLabInstanceIDLabel + "=" + instanceID,
		},
	}

	return dockerClient.ContainerList(ctx, types.ContainerListOptions{
		Filters: filters.NewArgs(filterPairs...),
	})
}

func shouldStopInternalProcess(controlLabel string) bool {
	return controlLabel == DBLabSyncLabel
}

func getControlContainerName(controlCont types.Container) string {
	return strings.Join(controlCont.Names, ", ")
}

// BuildHostConfig builds host config.
func BuildHostConfig(ctx context.Context, docker *client.Client, dataDir string,
	contConf map[string]interface{}) (*container.HostConfig, error) {
	hostOptions, err := ResourceOptions(contConf)
	if err != nil {
		return nil, err
	}

	hostConfig := &container.HostConfig{
		Resources: hostOptions.Resources,
		ShmSize:   hostOptions.ShmSize,
	}

	if err := tools.AddVolumesToHostConfig(ctx, docker, hostConfig, dataDir); err != nil {
		return nil, err
	}

	return hostConfig, nil
}

// ResourceOptions parses host config options.
func ResourceOptions(containerConfigs map[string]interface{}) (*container.HostConfig, error) {
	normalizedConfig := make(map[string]interface{}, len(containerConfigs))

	for configKey, configValue := range containerConfigs {
		normalizedKey := strings.ToLower(strings.ReplaceAll(configKey, "-", ""))

		// Convert human-readable string representing an amount of memory.
		if valueString, ok := configValue.(string); ok {
			ramInBytes, err := units.RAMInBytes(valueString)
			if err == nil {
				normalizedConfig[normalizedKey] = ramInBytes
				continue
			}
		}

		normalizedConfig[normalizedKey] = configValue
	}

	// Unmarshal twice because composite types do not unmarshal correctly: https://github.com/go-yaml/yaml/issues/63
	hostConfig := &container.HostConfig{}
	if err := options.Unmarshal(normalizedConfig, &hostConfig); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal container configuration options")
	}

	resources := container.Resources{}
	if err := options.Unmarshal(normalizedConfig, &resources); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal container configuration options")
	}

	hostConfig.Resources = resources

	return hostConfig, nil
}
