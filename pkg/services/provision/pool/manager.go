/*
2020 © Postgres.ai
*/

// Package pool provides components to work with storage pools.
package pool

import (
	"fmt"
	"os/user"

	"github.com/pkg/errors"

	"gitlab.com/postgres-ai/database-lab/v2/pkg/log"
	"gitlab.com/postgres-ai/database-lab/v2/pkg/services/provision/resources"
	"gitlab.com/postgres-ai/database-lab/v2/pkg/services/provision/runners"
	"gitlab.com/postgres-ai/database-lab/v2/pkg/services/provision/thinclones/lvm"
	"gitlab.com/postgres-ai/database-lab/v2/pkg/services/provision/thinclones/zfs"
)

// FSManager defines an interface to work different thin-clone managers.
type FSManager interface {
	Cloner
	Snapshotter
	StateReporter
	Pooler
}

// Cloner describes methods of clone management.
type Cloner interface {
	CreateClone(name, snapshotID string) error
	DestroyClone(name string) error
	ListClonesNames() ([]string, error)
}

// StateReporter describes methods of state reporting.
type StateReporter interface {
	GetSessionState(name string) (*resources.SessionState, error)
	GetDiskState() (*resources.Disk, error)
}

// Snapshotter describes methods of snapshot management.
type Snapshotter interface {
	CreateSnapshot(poolSuffix, dataStateAt string) (snapshotName string, err error)
	DestroySnapshot(snapshotName string) (err error)
	CleanupSnapshots(retentionLimit int) ([]string, error)
	GetSnapshots() ([]resources.Snapshot, error)
}

// Pooler describes methods for Pool providing.
type Pooler interface {
	Pool() *resources.Pool
}

// ManagerConfig defines thin-clone manager config.
type ManagerConfig struct {
	Pool              *resources.Pool
	PreSnapshotSuffix string
}

// NewManager defines constructor for thin-clone managers.
func NewManager(runner runners.Runner, config ManagerConfig) (FSManager, error) {
	var (
		manager FSManager
		err     error
	)

	switch config.Pool.Mode {
	case ZFS:
		osUser, err := user.Current()
		if err != nil {
			return nil, errors.Wrap(err, "failed to get current user")
		}

		manager = zfs.NewFSManager(runner, zfs.Config{
			Pool:              config.Pool,
			PreSnapshotSuffix: config.PreSnapshotSuffix,
			OSUsername:        osUser.Username,
		})

	case LVM:
		if manager, err = lvm.NewFSManager(runner, config.Pool); err != nil {
			return nil, errors.Wrap(err, "failed to initialize LVM thin-clone manager")
		}

	default:
		return nil, errors.New(fmt.Sprintf(`unsupported thin-clone manager specified: "%s"`, config.Pool.Mode))
	}

	log.Dbg(fmt.Sprintf(`Using "%s" thin-clone manager.`, config.Pool.Mode))

	return manager, nil
}
