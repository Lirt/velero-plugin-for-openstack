package cinder

import (
	"fmt"
	"math/rand"
	"strconv"

	"github.com/Lirt/velero-plugin-swift/src/utils"
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/blockstorage/v3/snapshots"
	"github.com/gophercloud/gophercloud/openstack/blockstorage/v3/volumes"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// BlockStore is a plugin for containing state for the Cinder Block Storage
type BlockStore struct {
	client    *gophercloud.ServiceClient
	provider  *gophercloud.ProviderClient
	config    map[string]string
	volumes   map[string]volumes.Volume
	snapshots map[string]snapshots.Snapshot
	log       logrus.FieldLogger
}

// NewBlockStore instantiates a Cinder Volume Snapshotter.
func NewBlockStore(log logrus.FieldLogger) *BlockStore {
	return &BlockStore{log: log}
}

var _ velero.VolumeSnapshotter = (*BlockStore)(nil)

// Init prepares the Cinder VolumeSnapshotter for usage using the provided map of
// configuration key-value pairs. It returns an error if the VolumeSnapshotter
// cannot be initialized from the provided config.
func (b *BlockStore) Init(config map[string]string) error {
	b.log.Infof("BlockStore.Init called", config)
	b.config = config

	// Make sure we don't overwrite data, now that we can re-initialize the plugin
	if b.volumes == nil {
		b.volumes = make(map[string]volumes.Volume)
	}
	if b.snapshots == nil {
		b.snapshots = make(map[string]snapshots.Snapshot)
	}

	// Authenticate to Openstack
	err := utils.Authenticate(&b.provider, b.log)
	if err != nil {
		return fmt.Errorf("failed to authenticate against openstack: %v", err)
	}

	if b.client == nil {
		region := utils.GetEnv("OS_REGION_NAME", "")
		b.client, err = openstack.NewBlockStorageV3(b.provider, gophercloud.EndpointOpts{
			Region: region,
		})
		if err != nil {
			return fmt.Errorf("failed to create cinder storage object: %v", err)
		}
	}

	return nil
}

// CreateVolumeFromSnapshot creates a new volume in the specified
// availability zone, initialized from the provided snapshot,
// and with the specified type.
// IOPS is ignored as it is not used in Cinder.
func (b *BlockStore) CreateVolumeFromSnapshot(snapshotName, volumeType, volumeAZ string, iops *int64) (string, error) {
	b.log.Infof("CreateVolumeFromSnapshot called", snapshotName, volumeType, volumeAZ)
	readyTimeout := 300
	snapshotID := b.snapshots[snapshotName].ID
	volumeName := fmt.Sprintf("%s.backup.%s", snapshotID, strconv.FormatUint(rand.Uint64(), 10))

	// Make sure snapshot is in ready state
	// Possible values for snapshot state:
	//   https://github.com/openstack/cinder/blob/master/api-ref/source/v3/volumes-v3-snapshots.inc#volume-snapshots-snapshots
	b.log.Infof("Waiting for snapshot to be in 'available' state", snapshotName, snapshotID, readyTimeout)

	err := snapshots.WaitForStatus(b.client, snapshotID, "available", readyTimeout)
	if err != nil {
		b.log.Errorf("snapshot didn't get into 'available' state within the time limit", snapshotName, snapshotID, readyTimeout)
		return "", err
	}
	b.log.Infof("Snapshot is in 'available' state", snapshotName, snapshotID)

	// Create Cinder Volume from snapshot (backup)
	b.log.Infof("Starting to create volume from snapshot")
	opts := volumes.CreateOpts{
		Description:      "Velero backup from snapshot",
		Name:             volumeName,
		VolumeType:       volumeType,
		AvailabilityZone: volumeAZ,
		SnapshotID:       snapshotID,
	}

	var cinderVolume *volumes.Volume
	cinderVolume, err = volumes.Create(b.client, opts).Extract()
	if err != nil {
		b.log.Errorf("failed to create volume from snapshot", snapshotName, snapshotID)
		return "", errors.WithStack(err)
	}
	b.log.Infof("Backup volume was created", cinderVolume.ID)

	// Remember the volume
	b.volumes[cinderVolume.ID] = volumes.Volume{
		Description:      "Velero backup from snapshot",
		VolumeType:       volumeType,
		AvailabilityZone: volumeAZ,
		SnapshotID:       snapshotID,
		ID:               cinderVolume.ID,
		Name:             volumeName,
	}

	return cinderVolume.ID, nil
}

// GetVolumeInfo returns the type the specified volume in the given availability zone.
// IOPS is not used as it is not supported by Cinder.
func (b *BlockStore) GetVolumeInfo(volumeID, volumeAZ string) (string, *int64, error) {
	b.log.Infof("GetVolumeInfo called", volumeID, volumeAZ)

	// Volume exists in list of volumes
	if vol, ok := b.volumes[volumeID]; ok {
		return vol.VolumeType, nil, nil
	}

	// Volume doesn't exist in list of volumes
	return "", nil, fmt.Errorf("volume %v not found", volumeID)
}

// IsVolumeReady Check if the volume is ready.
func (b *BlockStore) IsVolumeReady(volumeID, volumeAZ string) (ready bool, err error) {
	b.log.Infof("IsVolumeReady called", volumeID, volumeAZ)

	// If volume doesn't exist in list of volumes
	_, ok := b.volumes[volumeID]
	if !ok {
		return false, fmt.Errorf("volume %v doesn't exist in volumes map", volumeID)
	}

	// Get volume object from Cinder
	cinderVolume, err := volumes.Get(b.client, volumeID).Extract()
	if err != nil {
		b.log.Errorf("failed to get volume %v from Cinder", volumeID)
		return false, err
	}

	// These are ready states
	//   https://github.com/openstack/cinder/blob/master/api-ref/source/v3/volumes-v3-volumes.inc#volumes-volumes
	if cinderVolume.Status == "available" || cinderVolume.Status == "in-use" {
		return true, nil
	}

	// Volume is not in one of the "ready" states
	return false, fmt.Errorf("volume %v is not in ready state, the status is %v", volumeID, cinderVolume.Status)
}

// CreateSnapshot creates a snapshot of the specified volume, and applies any provided
// set of tags to the snapshot.
func (b *BlockStore) CreateSnapshot(volumeID, volumeAZ string, tags map[string]string) (string, error) {
	b.log.Infof("CreateSnapshot called", volumeID, volumeAZ, tags)
	snapshotName := fmt.Sprintf("%s.snap.%s", volumeID, strconv.FormatUint(rand.Uint64(), 10))

	b.log.Infof("Trying to create snapshot", snapshotName)
	opts := snapshots.CreateOpts{
		Name:        snapshotName,
		Description: "Velero snapshot",
		Metadata:    tags,
		VolumeID:    volumeID,
	}

	// Note: we will wait for snapshot to be in ready state in CreateVolumeForSnapshot()
	createResult, err := snapshots.Create(b.client, opts).Extract()
	if err != nil {
		return "", errors.WithStack(err)
	}
	snapshotID := createResult.ID

	// Remember the snapshot
	b.snapshots[snapshotName] = snapshots.Snapshot{
		Name:        snapshotName,
		ID:          snapshotID,
		Description: "Velero snapshot",
		Metadata:    tags,
		VolumeID:    volumeID,
	}

	b.log.Infof("Snapshot finished successfuly", snapshotName, snapshotID)
	return snapshotName, nil
}

// DeleteSnapshot deletes the specified volume snapshot.
func (b *BlockStore) DeleteSnapshot(snapshotName string) error {
	b.log.Infof("DeleteSnapshot called", snapshotName)
	snapshotID := b.snapshots[snapshotName].ID

	// Delete snapshot from Cinder
	b.log.Infof("Deleting Snapshot with ID", snapshotID)
	err := snapshots.Delete(b.client, snapshotID).ExtractErr()
	if err != nil {
		return errors.WithStack(err)
	}

	// Delete snapshot from list of snapshots
	delete(b.snapshots, snapshotID)

	return nil
}

// GetVolumeID returns the specific identifier for the PersistentVolume.
func (b *BlockStore) GetVolumeID(unstructuredPV runtime.Unstructured) (string, error) {
	b.log.Infof("GetVolumeID called", unstructuredPV)

	pv := new(v1.PersistentVolume)
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredPV.UnstructuredContent(), pv); err != nil {
		return "", errors.WithStack(err)
	}

	if pv.Spec.Cinder == nil {
		return "", nil
	}

	if pv.Spec.Cinder.VolumeID == "" {
		return "", errors.New("spec.cinder.volumeID not found")
	}

	return pv.Spec.Cinder.VolumeID, nil
}

// SetVolumeID sets the specific identifier for the PersistentVolume.
func (b *BlockStore) SetVolumeID(unstructuredPV runtime.Unstructured, volumeID string) (runtime.Unstructured, error) {
	b.log.Infof("SetVolumeID called", unstructuredPV, volumeID)

	pv := new(v1.PersistentVolume)
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredPV.UnstructuredContent(), pv); err != nil {
		return nil, errors.WithStack(err)
	}

	if pv.Spec.Cinder == nil {
		return nil, errors.New("spec.cinder not found")
	}

	pv.Spec.Cinder.VolumeID = volumeID

	res, err := runtime.DefaultUnstructuredConverter.ToUnstructured(pv)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &unstructured.Unstructured{Object: res}, nil
}
