package cinder

import (
	"fmt"
	"os"
	"strconv"

	"github.com/Lirt/velero-plugin-for-openstack/src/utils"
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/blockstorage/v3/snapshots"
	"github.com/gophercloud/gophercloud/openstack/blockstorage/v3/volumes"
	"github.com/sirupsen/logrus"
	velerovolumesnapshotter "github.com/vmware-tanzu/velero/pkg/plugin/velero/volumesnapshotter/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	defaultTimeout = "5m"
)

var (
	// a list of supported Cinder CSI drivers
	supportedDrivers = []string{
		// standard Cinder CSI driver
		"cinder.csi.openstack.org",
		// Huawei Cloud Cinder CSI driver
		"disk.csi.everest.io",
	}
	// active volume statuses
	//   https://github.com/openstack/cinder/blob/master/api-ref/source/v3/volumes-v3-volumes.inc#volumes-volumes
	volumeStatuses = []string{
		"available",
		"in-use",
	}
	// active snapshot statuses
	//   https://github.com/openstack/cinder/blob/master/api-ref/source/v3/volumes-v3-snapshots.inc#volume-snapshots-snapshots
	snapshotStatuses = []string{
		"available",
	}
)

// BlockStore is a plugin for containing state for the Cinder Block Storage
type BlockStore struct {
	client          *gophercloud.ServiceClient
	provider        *gophercloud.ProviderClient
	config          map[string]string
	volumeTimeout   int
	snapshotTimeout int
	log             logrus.FieldLogger
}

// NewBlockStore instantiates a Cinder Volume Snapshotter.
func NewBlockStore(log logrus.FieldLogger) *BlockStore {
	return &BlockStore{log: log}
}

var _ velerovolumesnapshotter.VolumeSnapshotter = (*BlockStore)(nil)

// Init prepares the Cinder VolumeSnapshotter for usage using the provided map of
// configuration key-value pairs. It returns an error if the VolumeSnapshotter
// cannot be initialized from the provided config.
func (b *BlockStore) Init(config map[string]string) error {
	b.log.WithFields(logrus.Fields{
		"config": config,
	}).Info("BlockStore.Init called")
	b.config = config

	// parse timeouts
	var err error
	b.volumeTimeout, err = utils.DurationToSeconds(utils.GetConf(b.config, "volumeTimeout", defaultTimeout))
	if err != nil {
		return fmt.Errorf("cannot parse time from volumeTimeout config variable: %w", err)
	}
	b.snapshotTimeout, err = utils.DurationToSeconds(utils.GetConf(b.config, "snapshotTimeout", defaultTimeout))
	if err != nil {
		return fmt.Errorf("cannot parse time from snapshotTimeout config variable: %w", err)
	}

	// Authenticate to OpenStack
	err = utils.Authenticate(&b.provider, "cinder", config, b.log)
	if err != nil {
		return fmt.Errorf("failed to authenticate against OpenStack in block storage plugin: %w", err)
	}

	// If we haven't set client before or we use multiple clouds - get new client
	if b.client == nil || config["cloud"] != "" {
		region, ok := os.LookupEnv("OS_REGION_NAME")
		if !ok {
			if config["region"] != "" {
				region = config["region"]
			} else {
				region = "RegionOne"
			}
		}
		b.client, err = openstack.NewBlockStorageV3(b.provider, gophercloud.EndpointOpts{
			Region: region,
		})
		if err != nil {
			return fmt.Errorf("failed to create cinder storage client: %w", err)
		}
		b.log.WithFields(logrus.Fields{
			"endpoint": b.client.Endpoint,
			"region":   region,
		}).Info("Successfully created block storage service client")
	}

	return nil
}

// CreateVolumeFromSnapshot creates a new volume in the specified
// availability zone, initialized from the provided snapshot and with the specified type.
// IOPS is ignored as it is not used in Cinder.
func (b *BlockStore) CreateVolumeFromSnapshot(snapshotID, volumeType, volumeAZ string, iops *int64) (string, error) {
	logWithFields := b.log.WithFields(logrus.Fields{
		"snapshotID":      snapshotID,
		"volumeType":      volumeType,
		"volumeAZ":        volumeAZ,
		"snapshotTimeout": b.snapshotTimeout,
		"volumeTimeout":   b.volumeTimeout,
	})
	logWithFields.Info("BlockStore.CreateVolumeFromSnapshot called")

	volumeName := fmt.Sprintf("%s.backup.%s", snapshotID, strconv.FormatUint(utils.Rand.Uint64(), 10))
	logWithFields.Info("Waiting for snapshot to be in 'available' state")

	snapshot, err := b.waitForSnapshotStatus(snapshotID, snapshotStatuses, b.snapshotTimeout)
	if err != nil {
		logWithFields.Error("snapshot didn't get into 'available' state within the time limit")
		return "", fmt.Errorf("snapshot %v didn't get into 'available' state within the time limit: %w", snapshotID, err)
	}
	logWithFields.Info("Snapshot is in 'available' state")

	// get original volume with its metadata
	originVolume, err := volumes.Get(b.client, snapshot.VolumeID).Extract()
	if err != nil {
		logWithFields.Error("failed to get volume from cinder")
		return "", fmt.Errorf("failed to get volume %v from cinder: %w", snapshot.VolumeID, err)
	}

	// Create Cinder Volume from snapshot (backup)
	logWithFields.Info("Starting to create volume from snapshot")
	opts := volumes.CreateOpts{
		Description:      "Velero backup from snapshot",
		Name:             volumeName,
		VolumeType:       volumeType,
		AvailabilityZone: volumeAZ,
		SnapshotID:       snapshotID,
		Metadata:         originVolume.Metadata,
	}

	volume, err := volumes.Create(b.client, opts).Extract()
	if err != nil {
		logWithFields.Error("failed to create volume from snapshot")
		return "", fmt.Errorf("failed to create volume %v from snapshot %v: %w", volumeName, snapshotID, err)
	}

	_, err = b.waitForVolumeStatus(volume.ID, volumeStatuses, b.volumeTimeout)
	if err != nil {
		logWithFields.Error("volume didn't get into 'available' state within the time limit")
		return "", fmt.Errorf("volume %v didn't get into 'available' state within the time limit: %w", volume.ID, err)
	}

	logWithFields.WithFields(logrus.Fields{
		"volumeID": volume.ID,
	}).Info("Backup volume was created")
	return volume.ID, nil
}

// GetVolumeInfo returns type of the specified volume in the given availability zone.
// IOPS is not used as it is not supported by Cinder.
func (b *BlockStore) GetVolumeInfo(volumeID, volumeAZ string) (string, *int64, error) {
	logWithFields := b.log.WithFields(logrus.Fields{
		"volumeID": volumeID,
		"volumeAZ": volumeAZ,
	})
	logWithFields.Info("BlockStore.GetVolumeInfo called")

	volume, err := volumes.Get(b.client, volumeID).Extract()
	if err != nil {
		logWithFields.Error("failed to get volume from cinder")
		return "", nil, fmt.Errorf("failed to get volume %v from cinder: %w", volumeID, err)
	}

	return volume.VolumeType, nil, nil
}

// IsVolumeReady Check if the volume is in one of the ready states.
func (b *BlockStore) IsVolumeReady(volumeID, volumeAZ string) (ready bool, err error) {
	logWithFields := b.log.WithFields(logrus.Fields{
		"volumeID": volumeID,
		"volumeAZ": volumeAZ,
	})
	logWithFields.Info("BlockStore.IsVolumeReady called")

	// Get volume object from Cinder
	volume, err := volumes.Get(b.client, volumeID).Extract()
	if err != nil {
		logWithFields.Error("failed to get volume from cinder")
		return false, fmt.Errorf("failed to get volume %v from cinder: %w", volumeID, err)
	}

	if utils.SliceContains(volumeStatuses, volume.Status) {
		return true, nil
	}

	// Volume is not in one of the "ready" states
	return false, fmt.Errorf("volume %v is not in ready state, the status is %v", volumeID, volume.Status)
}

// CreateSnapshot creates a snapshot of the specified volume, and applies any provided
// set of tags to the snapshot.
func (b *BlockStore) CreateSnapshot(volumeID, volumeAZ string, tags map[string]string) (string, error) {
	snapshotName := fmt.Sprintf("%s.snap.%s", volumeID, strconv.FormatUint(utils.Rand.Uint64(), 10))
	logWithFields := b.log.WithFields(logrus.Fields{
		"snapshotName":    snapshotName,
		"volumeID":        volumeID,
		"volumeAZ":        volumeAZ,
		"tags":            tags,
		"snapshotTimeout": b.snapshotTimeout,
	})
	logWithFields.Info("BlockStore.CreateSnapshot called")

	originVolume, err := volumes.Get(b.client, volumeID).Extract()
	if err != nil {
		logWithFields.Error("failed to get volume from cinder")
		return "", fmt.Errorf("failed to get volume %v from cinder: %w", volumeID, err)
	}

	opts := snapshots.CreateOpts{
		Name:        snapshotName,
		Description: "Velero snapshot",
		Metadata:    utils.Merge(originVolume.Metadata, tags),
		VolumeID:    volumeID,
		Force:       true,
	}

	snapshot, err := snapshots.Create(b.client, opts).Extract()
	if err != nil {
		logWithFields.Error("failed to create snapshot from volume")
		return "", fmt.Errorf("failed to create snapshot %v from volume %v: %w", snapshotName, volumeID, err)
	}

	_, err = b.waitForSnapshotStatus(snapshot.ID, snapshotStatuses, b.snapshotTimeout)
	if err != nil {
		logWithFields.Error("snapshot didn't get into 'available' state within the time limit")
		return "", fmt.Errorf("snapshot %v didn't get into 'available' state within the time limit: %w", snapshot.ID, err)
	}
	logWithFields.Info("Snapshot is in 'available' state")

	logWithFields.WithFields(logrus.Fields{
		"snapshotID": snapshot.ID,
	}).Info("Snapshot finished successfuly")
	return snapshot.ID, nil
}

// DeleteSnapshot deletes the specified volume snapshot.
func (b *BlockStore) DeleteSnapshot(snapshotID string) error {
	logWithFields := b.log.WithFields(logrus.Fields{
		"snapshotID": snapshotID,
	})
	logWithFields.Info("BlockStore.DeleteSnapshot called")

	// Delete snapshot from Cinder
	err := snapshots.Delete(b.client, snapshotID).ExtractErr()
	if err != nil {
		if _, ok := err.(gophercloud.ErrDefault404); ok {
			logWithFields.Info("snapshot is already deleted")
			return nil
		}
		logWithFields.Error("failed to delete snapshot")
		return fmt.Errorf("failed to delete snapshot %v: %w", snapshotID, err)
	}

	return nil
}

// GetVolumeID returns the specific identifier for the PersistentVolume.
func (b *BlockStore) GetVolumeID(unstructuredPV runtime.Unstructured) (string, error) {
	logWithFields := b.log.WithFields(logrus.Fields{
		"unstructuredPV": unstructuredPV,
	})
	logWithFields.Info("BlockStore.GetVolumeID called")

	pv := new(v1.PersistentVolume)
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredPV.UnstructuredContent(), pv); err != nil {
		return "", fmt.Errorf("failed to convert from unstructured PV: %w", err)
	}

	if pv.Spec.Cinder != nil {
		return pv.Spec.Cinder.VolumeID, nil
	}

	if pv.Spec.CSI == nil {
		return "", nil
	}

	if utils.SliceContains(supportedDrivers, pv.Spec.CSI.Driver) {
		return pv.Spec.CSI.VolumeHandle, nil
	}

	b.log.Infof("Unable to handle CSI driver: %s", pv.Spec.CSI.Driver)

	return "", nil
}

// SetVolumeID sets the specific identifier for the PersistentVolume.
func (b *BlockStore) SetVolumeID(unstructuredPV runtime.Unstructured, volumeID string) (runtime.Unstructured, error) {
	logWithFields := b.log.WithFields(logrus.Fields{
		"unstructuredPV": unstructuredPV,
		"volumeID":       volumeID,
	})
	logWithFields.Info("BlockStore.SetVolumeID called")

	pv := new(v1.PersistentVolume)
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredPV.UnstructuredContent(), pv); err != nil {
		return nil, fmt.Errorf("failed to convert from unstructured PV: %w", err)
	}

	if pv.Spec.Cinder != nil {
		pv.Spec.Cinder.VolumeID = volumeID
	} else if pv.Spec.CSI != nil && utils.SliceContains(supportedDrivers, pv.Spec.CSI.Driver) {
		pv.Spec.CSI.VolumeHandle = volumeID
	} else {
		return nil, fmt.Errorf("persistent volume is missing 'spec.cinder.volumeID' or PV driver ('spec.csi.driver') doesn't match supported drivers (%v)", supportedDrivers)
	}

	res, err := runtime.DefaultUnstructuredConverter.ToUnstructured(pv)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to unstructured PV: %w", err)
	}

	return &unstructured.Unstructured{Object: res}, nil
}

func (b *BlockStore) waitForVolumeStatus(id string, statuses []string, secs int) (current *volumes.Volume, err error) {
	return current, gophercloud.WaitFor(secs, func() (bool, error) {
		current, err = volumes.Get(b.client, id).Extract()
		if err != nil {
			return false, err
		}

		if utils.SliceContains(statuses, current.Status) {
			return true, nil
		}

		return false, nil
	})
}

func (b *BlockStore) waitForSnapshotStatus(id string, statuses []string, secs int) (current *snapshots.Snapshot, err error) {
	return current, gophercloud.WaitFor(secs, func() (bool, error) {
		current, err = snapshots.Get(b.client, id).Extract()
		if err != nil {
			return false, err
		}

		if utils.SliceContains(statuses, current.Status) {
			return true, nil
		}

		return false, nil
	})
}
