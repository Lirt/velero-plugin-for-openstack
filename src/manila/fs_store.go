package manila

import (
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"time"

	"github.com/Lirt/velero-plugin-for-openstack/src/utils"
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/sharedfilesystems/v2/shareaccessrules"
	"github.com/gophercloud/gophercloud/openstack/sharedfilesystems/v2/shares"
	"github.com/gophercloud/gophercloud/openstack/sharedfilesystems/v2/snapshots"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	velerovolumesnapshotter "github.com/vmware-tanzu/velero/pkg/plugin/velero/volumesnapshotter/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// FSStore is a plugin for containing state for the Manila Shared Filesystem
type FSStore struct {
	client   *gophercloud.ServiceClient
	provider *gophercloud.ProviderClient
	config   map[string]string
	log      logrus.FieldLogger
}

// NewFSStore instantiates a Manila Shared Filesystem Snapshotter.
func NewFSStore(log logrus.FieldLogger) *FSStore {
	return &FSStore{log: log}
}

var _ velerovolumesnapshotter.VolumeSnapshotter = (*FSStore)(nil)

// Init prepares the Manila VolumeSnapshotter for usage using the provided map of
// configuration key-value pairs. It returns an error if the VolumeSnapshotter
// cannot be initialized from the provided config.
func (b *FSStore) Init(config map[string]string) error {
	b.log.WithFields(logrus.Fields{
		"config": config,
	}).Info("FSStore.Init called")
	b.config = config

	// Authenticate to Openstack
	err := utils.Authenticate(&b.provider, "manila", config, b.log)
	if err != nil {
		return fmt.Errorf("failed to authenticate against OpenStack in shared filesystem plugin: %w", err)
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
		b.client, err = openstack.NewSharedFileSystemV2(b.provider, gophercloud.EndpointOpts{
			Region: region,
		})
		if err != nil {
			return fmt.Errorf("failed to create manila storage client: %w", err)
		}

		// TODO: rewise
		b.client.Microversion = "2.57"

		b.log.WithFields(logrus.Fields{
			"endpoint": b.client.Endpoint,
			"region":   region,
		}).Info("Successfully created shared filesystem service client")
	}

	return nil
}

// CreateVolumeFromSnapshot creates a new volume in the specified
// availability zone, initialized from the provided snapshot and with the specified type.
// IOPS is ignored as it is not used in Manila.
func (b *FSStore) CreateVolumeFromSnapshot(snapshotID, volumeType, volumeAZ string, iops *int64) (string, error) {
	b.log.Infof("CreateVolumeFromSnapshot called", snapshotID, volumeType, volumeAZ)

	volumeName := fmt.Sprintf("%s.backup.%s", snapshotID, strconv.FormatUint(rand.Uint64(), 10))

	sp, err := snapshots.Get(b.client, snapshotID).Extract()
	if err != nil {
		b.log.Error(err)
	}

	// Snapshot should be already in ready state because we wait for it when creating
	if sp.Status == "available" {
		b.log.Infof("Snapshot is in 'available' state", snapshotID)
	} else {
		b.log.Errorf("Snapshot is not in 'available' state", snapshotID)
		return "", err
	}

	// Create Manila Volume from snapshot (backup)
	b.log.Infof("Starting to create volume from snapshot")

	createOpts := &shares.CreateOpts{
		AvailabilityZone: volumeAZ,
		Name:             volumeName,
		SnapshotID:       sp.ID,
		ShareProto:       sp.ShareProto,
		Size:             sp.Size,
	}
	share, err := shares.Create(b.client, createOpts).Extract()

	if err != nil {
		b.log.Errorf("failed to create volume from snapshot", snapshotID)
		return "", errors.WithStack(err)
	}

	// Wait the share to be ready
	ready, err := b.IsVolumeReady(share.ID, volumeAZ)
	shareReadyTimeout := 300
	for shareReadyTimeout > 0 {
		time.Sleep(5 * time.Second)
		shareReadyTimeout -= 5
		ready, err = b.IsVolumeReady(share.ID, volumeAZ)
		if err != nil {
			b.log.Error(err)
		}

		if ready {
			break
		}
	}

	if ready {
		b.log.Infof("Backup volume was created", volumeName, share.ID)
	} else {
		b.log.Errorf("share didn't get into 'available' state within the time limit", share.ID, shareReadyTimeout)
		return "", err
	}

	return share.ID, nil
}

func (b *FSStore) GetVolumeInfo(volumeID, volumeAZ string) (string, *int64, error) {
	b.log.Infof("GetVolumeInfo called", volumeID, volumeAZ)

	volume, err := shares.Get(b.client, volumeID).Extract()
	if err != nil {
		b.log.Errorf("failed to get volume %v from Manila: %v", volumeID, err)
		return "", nil, fmt.Errorf("failed to get volume %v", volumeID)
	}

	return volume.VolumeType, nil, nil
}

// IsVolumeReady Check if the volume is in one of the ready states.
func (b *FSStore) IsVolumeReady(volumeID, volumeAZ string) (ready bool, err error) {
	b.log.Infof("IsVolumeReady called", volumeID, volumeAZ)

	// Get volume object from Manila
	manilaVolume, err := shares.Get(b.client, volumeID).Extract()
	if err != nil {
		b.log.Errorf("failed to get volume %v from Manila", volumeID)
		return false, err
	}

	// Ready states:
	//   https://github.com/openstack/manila/blob/master/api-ref/source/shares.inc
	if manilaVolume.Status == "available" {
		return true, nil
	}

	// Volume is not in one of the "ready" states
	return false, fmt.Errorf("volume %v is not in ready state, the status is %v", volumeID, manilaVolume.Status)
}

// CreateSnapshot creates a snapshot of the specified volume, and does NOT apply any provided
// set of tags to the snapshot.
func (b *FSStore) CreateSnapshot(volumeID, volumeAZ string, tags map[string]string) (string, error) {
	b.log.Infof("CreateSnapshot called", volumeID, volumeAZ, tags)
	snapshotName := fmt.Sprintf("%s.snap.%s", volumeID, strconv.FormatUint(rand.Uint64(), 10))

	b.log.Infof("Trying to create snapshot", snapshotName)

	// NOTE: Because the `Description` field can only support up to 255 bytes,
	//       it seems to be too small to have `tags` stored in the snapshot
	description := "velero backup"
	opts := snapshots.CreateOpts{
		Name:               snapshotName,
		Description:        description,
		ShareID:            volumeID,
		DisplayName:        snapshotName,
		DisplayDescription: description,
	}

	createResult, err := snapshots.Create(b.client, opts).Extract()
	if err != nil {
		b.log.Error(err)
		return "", errors.WithStack(err)
	}
	snapshotID := createResult.ID

	var snapshotReadyTimeout int
	snapshotReadyTimeout = 300
	// Make sure snapshot is in ready state
	// Possible values for snapshot state:
	//   https://github.com/openstack/manila/blob/master/api-ref/source/snapshots.inc
	b.log.Infof("Waiting for snapshot to be in 'available' state", snapshotID, snapshotReadyTimeout)

	sp, err := snapshots.Get(b.client, snapshotID).Extract()
	if err != nil {
		b.log.Error(err)
	}
	for sp.Status != "available" && snapshotReadyTimeout > 0 {
		time.Sleep(5 * time.Second)
		snapshotReadyTimeout -= 5
		sp, err = snapshots.Get(b.client, snapshotID).Extract()
		if err != nil {
			b.log.Error(err)
		}
	}
	if sp.Status == "available" {
		b.log.Infof("Snapshot is in 'available' state", snapshotID)
	} else {
		b.log.Errorf("snapshot didn't get into 'available' state within the time limit", snapshotID, snapshotReadyTimeout)
		return "", err
	}

	b.log.Infof("Snapshot finished successfuly", snapshotName, snapshotID)
	return snapshotID, nil
}

// DeleteSnapshot deletes the specified volume snapshot.
func (b *FSStore) DeleteSnapshot(snapshotID string) error {
	b.log.Infof("DeleteSnapshot called", snapshotID)

	// Delete snapshot from Manila
	b.log.Infof("Deleting Snapshot with ID", snapshotID)
	err := snapshots.Delete(b.client, snapshotID).ExtractErr()
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// GetVolumeID returns the specific identifier for the PersistentVolume.
func (b *FSStore) GetVolumeID(unstructuredPV runtime.Unstructured) (string, error) {
	b.log.Infof("GetVolumeID called", unstructuredPV)

	pv := new(v1.PersistentVolume)
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredPV.UnstructuredContent(), pv); err != nil {
		return "", errors.WithStack(err)
	}

	var volumeID string

	if pv.Spec.CSI.Driver == "nfs.manila.csi.openstack.org" {
		volumeID = pv.Spec.CSI.VolumeHandle
	}

	if volumeID == "" {
		return "", errors.New("volumeID not found")
	}

	return volumeID, nil
}

// SetVolumeID sets the specific identifier for the PersistentVolume.
func (b *FSStore) SetVolumeID(unstructuredPV runtime.Unstructured, volumeID string) (runtime.Unstructured, error) {
	b.log.Infof("SetVolumeID called", unstructuredPV, volumeID)

	pv := new(v1.PersistentVolume)
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredPV.UnstructuredContent(), pv); err != nil {
		return nil, errors.WithStack(err)
	}

	if pv.Spec.CSI.Driver == "nfs.manila.csi.openstack.org" {
		pv.Spec.CSI.VolumeHandle = volumeID
		pv.Spec.CSI.VolumeAttributes["shareID"] = volumeID

		// To determine the share access to be created, we need to find along this path
		// new share -> snapshot -> old share -> old share's share access

		share, err := shares.Get(b.client, volumeID).Extract()
		if err != nil {
			b.log.Error(err)
			return nil, errors.WithStack(err)
		}

		snapshot, err := snapshots.Get(b.client, share.SnapshotID).Extract()
		if err != nil {
			b.log.Error(err)
			return nil, errors.WithStack(err)
		}

		originShare, err := shares.Get(b.client, snapshot.ShareID).Extract()
		if err != nil {
			b.log.Error(err)
			return nil, errors.WithStack(err)
		}

		rules, err := shareaccessrules.List(b.client, originShare.ID).Extract()
		if err != nil {
			b.log.Error(err)
			return nil, errors.WithStack(err)
		}

		if len(rules) < 1 {
			b.log.Error("No access rules found in the origin share %v", originShare.ID)
			return nil, errors.WithStack(err)
		}

		// Grant the first access as share access
		grantAccessOpts := &shares.GrantAccessOpts{
			AccessType:  rules[0].AccessType,
			AccessTo:    rules[0].AccessTo,
			AccessLevel: rules[0].AccessLevel,
		}
		accessRignt, err := shares.GrantAccess(b.client, volumeID, grantAccessOpts).Extract()
		if err != nil {
			b.log.Error(err)
			return nil, errors.WithStack(err)
		}

		pv.Spec.CSI.VolumeAttributes["shareAccessID"] = accessRignt.ID

	} else {
		return nil, errors.New("spec.csi for manila driver not found")
	}

	res, err := runtime.DefaultUnstructuredConverter.ToUnstructured(pv)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &unstructured.Unstructured{Object: res}, nil
}
