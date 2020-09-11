/*
2020 © Postgres.ai
*/

// Package docker provides an interface to work with Docker containers.
package docker

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/pkg/errors"
	"github.com/shirou/gopsutil/host"

	"gitlab.com/postgres-ai/database-lab/pkg/retrieval/engine/postgres/tools"
	"gitlab.com/postgres-ai/database-lab/pkg/services/provision/resources"
	"gitlab.com/postgres-ai/database-lab/pkg/services/provision/runners"
)

const (
	labelClone = "dblab_clone"
)

// RunContainer runs specified container.
func RunContainer(r runners.Runner, c *resources.AppConfig) (string, error) {
	hostInfo, err := host.Info()
	if err != nil {
		return "", errors.Wrap(err, "failed to get host info")
	}

	// Directly mount PGDATA if Database Lab is running without any virtualization.
	volumes := []string{fmt.Sprintf("--volume %s:%s", c.DataDir(), c.DataDir())}

	if hostInfo.VirtualizationRole == "guest" {
		// Build custom mounts rely on mounts of the Database Lab instance if it's running inside Docker container.
		// We cannot use --volumes-from because it removes the ZFS mount point.
		volumes, err = buildMountVolumes(r, c, hostInfo.Hostname)
		if err != nil {
			return "", errors.Wrap(err, "failed to detect container volumes")
		}
	}

	if err := createSocketCloneDir(c.UnixSocketCloneDir); err != nil {
		return "", errors.Wrap(err, "failed to create socket clone directory")
	}

	dockerRunCmd := strings.Join([]string{
		"docker run",
		"--name", c.CloneName,
		"--detach",
		"--publish", strconv.Itoa(int(c.Port)) + ":5432",
		"--env", "PGDATA=" + c.DataDir(),
		strings.Join(volumes, " "),
		"--label", labelClone,
		"--label", c.ClonePool,
		c.DockerImage,
		"-k", c.UnixSocketCloneDir,
	}, " ")

	return r.Run(dockerRunCmd, true)
}

func buildMountVolumes(r runners.Runner, c *resources.AppConfig, containerID string) ([]string, error) {
	inspectCmd := "docker inspect -f '{{ json .Mounts }}' " + containerID

	var mountPoints []types.MountPoint

	out, err := r.Run(inspectCmd, true)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get container mounts")
	}

	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &mountPoints); err != nil {
		return nil, errors.Wrap(err, "failed to interpret mount paths")
	}

	mounts := tools.GetMountsFromMountPoints(c.MountDir, c.DataDir(), mountPoints)
	volumes := make([]string, 0, len(mounts))

	for _, mount := range mounts {
		// Add extra mount for socket directories.
		if mount.Target == c.DataDir() && strings.HasPrefix(c.UnixSocketCloneDir, c.MountDir) {
			volumes = append(volumes, buildSocketMount(c, mount.Source))
		}

		volume := fmt.Sprintf("--volume %s:%s", mount.Source, mount.Target)

		if mount.BindOptions != nil && mount.BindOptions.Propagation != "" {
			volume += ":" + string(mount.BindOptions.Propagation)
		}

		volumes = append(volumes, volume)
	}

	return volumes, nil
}

// buildSocketMount builds a socket directory mounting rely on dataDir mounting.
func buildSocketMount(c *resources.AppConfig, hostDataDir string) string {
	socketPath := strings.TrimPrefix(c.UnixSocketCloneDir, c.MountDir)
	dataPath := strings.TrimPrefix(c.DataDir(), c.MountDir)
	externalMount := strings.TrimSuffix(hostDataDir, dataPath)
	hostSocketDir := path.Join(externalMount, socketPath)

	return fmt.Sprintf(" --volume %s:%s:rshared", hostSocketDir, c.UnixSocketCloneDir)
}

func createSocketCloneDir(socketCloneDir string) error {
	if err := os.RemoveAll(socketCloneDir); err != nil {
		return err
	}

	if err := os.MkdirAll(socketCloneDir, 0777); err != nil {
		return err
	}

	if err := os.Chmod(socketCloneDir, 0777); err != nil {
		return err
	}

	return nil
}

// StopContainer stops specified container.
func StopContainer(r runners.Runner, c *resources.AppConfig) (string, error) {
	dockerStopCmd := "docker container stop " + c.CloneName

	return r.Run(dockerStopCmd, true)
}

// RemoveContainer removes specified container.
func RemoveContainer(r runners.Runner, c *resources.AppConfig) (string, error) {
	dockerRemoveCmd := "docker container rm --force " + c.CloneName

	return r.Run(dockerRemoveCmd, true)
}

// ListContainers lists containers.
func ListContainers(r runners.Runner, clonePool string) ([]string, error) {
	dockerListCmd := fmt.Sprintf(`docker container ls --filter "label=%s" --filter "label=%s" --all --quiet`,
		labelClone, clonePool)

	out, err := r.Run(dockerListCmd, true)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list containers")
	}

	out = strings.TrimSpace(out)
	if len(out) == 0 {
		return []string{}, nil
	}

	return strings.Split(out, "\n"), nil
}

// GetLogs gets logs from specified container.
func GetLogs(r runners.Runner, c *resources.AppConfig, sinceRelMins uint) (string, error) {
	dockerLogsCmd := "docker logs " + c.CloneName + " " +
		"--since " + strconv.FormatUint(uint64(sinceRelMins), 10) + "m " +
		"--timestamps"

	return r.Run(dockerLogsCmd, true)
}

// Exec executes command on specified container.
func Exec(r runners.Runner, c *resources.AppConfig, cmd string) (string, error) {
	dockerExecCmd := "docker exec " + c.CloneName + " " + cmd

	return r.Run(dockerExecCmd, true)
}

// ImageExists checks existence of Docker image.
func ImageExists(r runners.Runner, dockerImage string) (bool, error) {
	dockerListImagesCmd := "docker images " + dockerImage + " --quiet"

	out, err := r.Run(dockerListImagesCmd, true)
	if err != nil {
		return false, errors.Wrap(err, "failed to list images")
	}

	return len(strings.TrimSpace(out)) > 0, nil
}

// PullImage pulls Docker image from DockerHub registry.
func PullImage(r runners.Runner, dockerImage string) error {
	dockerPullImageCmd := "docker pull " + dockerImage

	_, err := r.Run(dockerPullImageCmd, true)
	if err != nil {
		return errors.Wrap(err, "failed to pull images")
	}

	return err
}
