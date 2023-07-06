package manila

import (
	"fmt"
	"os"
	"strconv"

	"github.com/Lirt/velero-plugin-for-openstack/src/utils"
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/sharedfilesystems/apiversions"
	"github.com/gophercloud/gophercloud/openstack/sharedfilesystems/v2/shareaccessrules"
	"github.com/gophercloud/gophercloud/openstack/sharedfilesystems/v2/shares"
	"github.com/gophercloud/gophercloud/openstack/sharedfilesystems/v2/snapshots"
	"github.com/sirupsen/logrus"
	velerovolumesnapshotter "github.com/vmware-tanzu/velero/pkg/plugin/velero/volumesnapshotter/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	defaultCsiManilaDriverName = "nfs.manila.csi.openstack.org"
	minSupportedMicroversion   = "2.7"
	getAccessRulesMicroversion = "2.45"
	shareReadyTimeout          = 300
	snapshotReadyTimeout       = 300
)

var (
	// active share statuses
	//   https://github.com/openstack/manila/blob/master/api-ref/source/shares.inc#shares
	shareStatuses = []string{
		"available",
	}
	// active snapshot statuses
	//   https://github.com/openstack/manila/blob/master/api-ref/source/snapshots.inc#share-snapshots
	snapshotStatuses = []string{
		"available",
	}
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

	// set default Manila CSI driver name
	b.config["driver"] = utils.GetConf(b.config, "driver", defaultCsiManilaDriverName)

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

		logWithFields := b.log.WithFields(logrus.Fields{
			"endpoint": b.client.Endpoint,
			"region":   region,
		})

		// set minimum supported Manila API microversion by default
		b.client.Microversion = minSupportedMicroversion
		if mv, err := b.getManilaMicroversion(); err != nil {
			logWithFields.Warningf("Failed to obtain supported Manila microversions (using the default one: %v): %v", b.client.Microversion, err)
		} else {
			ok, err := utils.CompareMicroversions("lte", getAccessRulesMicroversion, mv)
			if err != nil {
				logWithFields.Warningf("Failed to compare supported Manila microversions (using the default one: %v): %v", b.client.Microversion, err)
			}

			if ok {
				b.client.Microversion = getAccessRulesMicroversion
				logWithFields.Infof("Setting the supported %v microversion", b.client.Microversion)
			}
		}

		logWithFields.Info("Successfully created shared filesystem service client")
	}

	return nil
}

// CreateVolumeFromSnapshot creates a new volume in the specified
// availability zone, initialized from the provided snapshot and with the specified type.
// IOPS is ignored as it is not used in Manila.
func (b *FSStore) CreateVolumeFromSnapshot(snapshotID, volumeType, volumeAZ string, iops *int64) (string, error) {
	logWithFields := b.log.WithFields(logrus.Fields{
		"snapshotID":           snapshotID,
		"volumeType":           volumeType,
		"volumeAZ":             volumeAZ,
		"shareReadyTimeout":    shareReadyTimeout,
		"snapshotReadyTimeout": snapshotReadyTimeout,
	})
	logWithFields.Info("FSStore.CreateVolumeFromSnapshot called")

	volumeName := fmt.Sprintf("%s.backup.%s", snapshotID, strconv.FormatUint(utils.Rand.Uint64(), 10))
	logWithFields.Info("Waiting for snapshot to be in 'available' status")

	snapshot, err := b.waitForSnapshotStatus(snapshotID, snapshotStatuses, snapshotReadyTimeout)
	if err != nil {
		logWithFields.Error("snapshot didn't get into 'available' status within the time limit")
		return "", fmt.Errorf("snapshot %v didn't get into 'available' status within the time limit: %w", snapshotID, err)
	}
	logWithFields.Info("Snapshot is in 'available' status")

	// get original share with its metadata
	originShare, err := shares.Get(b.client, snapshot.ShareID).Extract()
	if err != nil {
		logWithFields.Errorf("failed to get original share %v from manila", snapshot.ShareID)
		return "", fmt.Errorf("failed to get original share %v from manila: %w", snapshot.ShareID, err)
	}

	// get original share access rule
	rule, err := b.getShareAccessRule(logWithFields, snapshot.ShareID)
	if err != nil {
		return "", err
	}

	// Create Manila Share from snapshot (backup)
	logWithFields.Infof("Starting to create share from snapshot")
	opts := &shares.CreateOpts{
		ShareProto:       snapshot.ShareProto,
		Size:             snapshot.Size,
		AvailabilityZone: volumeAZ,
		Name:             volumeName,
		Description:      "Velero backup from snapshot",
		SnapshotID:       snapshotID,
		Metadata:         originShare.Metadata,
	}
	share, err := shares.Create(b.client, opts).Extract()
	if err != nil {
		logWithFields.Errorf("failed to create share from snapshot")
		return "", fmt.Errorf("failed to create share %v from snapshot %v: %w", volumeName, snapshotID, err)
	}

	logWithFields.Info("Waiting for share to be in 'available' status")

	_, err = b.waitForShareStatus(share.ID, shareStatuses, shareReadyTimeout)
	if err != nil {
		logWithFields.Error("share didn't get into 'available' status within the time limit")
		return "", fmt.Errorf("share %v didn't get into 'available' status within the time limit: %w", share.ID, err)
	}

	// grant the only one supported share access from the original share
	accessOpts := &shares.GrantAccessOpts{
		AccessType:  rule.AccessType,
		AccessTo:    rule.AccessTo,
		AccessLevel: rule.AccessLevel,
	}
	shareAccess, err := shares.GrantAccess(b.client, share.ID, accessOpts).Extract()
	if err != nil {
		logWithFields.Error("failed to grant an access to manila share")
		return "", fmt.Errorf("failed to grant an access to manila share %v: %w", share.ID, err)
	}

	logWithFields.WithFields(logrus.Fields{
		"shareID":       share.ID,
		"shareAccessID": shareAccess.ID,
	}).Info("Backup share was created")
	return share.ID, nil
}

// GetVolumeInfo returns type of the specified volume in the given availability zone.
// IOPS is not used as it is not supported by Manila.
func (b *FSStore) GetVolumeInfo(volumeID, volumeAZ string) (string, *int64, error) {
	logWithFields := b.log.WithFields(logrus.Fields{
		"volumeID": volumeID,
		"volumeAZ": volumeAZ,
	})
	logWithFields.Info("FSStore.GetVolumeInfo called")

	share, err := shares.Get(b.client, volumeID).Extract()
	if err != nil {
		logWithFields.Error("failed to get share from manila")
		return "", nil, fmt.Errorf("failed to get share %v from manila: %w", volumeID, err)
	}

	return share.VolumeType, nil, nil
}

// IsVolumeReady Check if the volume is in one of the available statuses.
func (b *FSStore) IsVolumeReady(volumeID, volumeAZ string) (ready bool, err error) {
	logWithFields := b.log.WithFields(logrus.Fields{
		"volumeID": volumeID,
		"volumeAZ": volumeAZ,
	})
	logWithFields.Info("FSStore.IsVolumeReady called")

	// Get share object from Manila
	share, err := shares.Get(b.client, volumeID).Extract()
	if err != nil {
		logWithFields.Error("failed to get share from manila")
		return false, fmt.Errorf("failed to get share %v from manila: %w", volumeID, err)
	}

	if utils.SliceContains(shareStatuses, share.Status) {
		return true, nil
	}

	// Share is not in one of the "available" statuses
	return false, fmt.Errorf("share %v is not in available status, the status is %v", volumeID, share.Status)
}

// CreateSnapshot creates a snapshot of the specified volume, and does NOT
// apply any provided set of tags to the snapshot.
func (b *FSStore) CreateSnapshot(volumeID, volumeAZ string, tags map[string]string) (string, error) {
	snapshotName := fmt.Sprintf("%s.snap.%s", volumeID, strconv.FormatUint(utils.Rand.Uint64(), 10))
	logWithFields := b.log.WithFields(logrus.Fields{
		"snapshotName":         snapshotName,
		"volumeID":             volumeID,
		"volumeAZ":             volumeAZ,
		"tags":                 tags,
		"snapshotReadyTimeout": snapshotReadyTimeout,
	})
	logWithFields.Info("FSStore.CreateSnapshot called")

	opts := snapshots.CreateOpts{
		Name:        snapshotName,
		Description: "Velero snapshot",
		ShareID:     volumeID,
		// TODO: add Metadata once https://github.com/gophercloud/gophercloud/issues/2660 is merged
	}
	snapshot, err := snapshots.Create(b.client, opts).Extract()
	if err != nil {
		logWithFields.Error("failed to create snapshot from share")
		return "", fmt.Errorf("failed to create snapshot %v from share %v: %w", snapshotName, volumeID, err)
	}

	_, err = b.waitForSnapshotStatus(snapshot.ID, snapshotStatuses, snapshotReadyTimeout)
	if err != nil {
		logWithFields.Error("snapshot didn't get into 'available' status within the time limit")
		return "", fmt.Errorf("snapshot %v didn't get into 'available' status within the time limit: %w", snapshot.ID, err)
	}
	logWithFields.Info("Snapshot is in 'available' status")

	logWithFields.WithFields(logrus.Fields{
		"snapshotID": snapshot.ID,
	}).Info("Snapshot finished successfuly")
	return snapshot.ID, nil
}

// DeleteSnapshot deletes the specified volume snapshot.
func (b *FSStore) DeleteSnapshot(snapshotID string) error {
	logWithFields := b.log.WithFields(logrus.Fields{
		"snapshotID": snapshotID,
	})
	logWithFields.Info("FSStore.DeleteSnapshot called")

	// Delete snapshot from Manila
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
func (b *FSStore) GetVolumeID(unstructuredPV runtime.Unstructured) (string, error) {
	logWithFields := b.log.WithFields(logrus.Fields{
		"unstructuredPV": unstructuredPV,
	})
	logWithFields.Info("FSStore.GetVolumeID called")

	pv := new(v1.PersistentVolume)
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredPV.UnstructuredContent(), pv); err != nil {
		return "", fmt.Errorf("failed to convert from unstructured PV: %w", err)
	}

	if pv.Spec.CSI == nil {
		return "", nil
	}

	if pv.Spec.CSI.Driver == b.config["driver"] {
		return pv.Spec.CSI.VolumeHandle, nil
	}

	b.log.Infof("Unable to handle CSI driver: %s", pv.Spec.CSI.Driver)

	return "", nil
}

// SetVolumeID sets the specific identifier for the PersistentVolume.
func (b *FSStore) SetVolumeID(unstructuredPV runtime.Unstructured, volumeID string) (runtime.Unstructured, error) {
	logWithFields := b.log.WithFields(logrus.Fields{
		"unstructuredPV": unstructuredPV,
		"volumeID":       volumeID,
	})
	logWithFields.Info("FSStore.SetVolumeID called")

	pv := new(v1.PersistentVolume)
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredPV.UnstructuredContent(), pv); err != nil {
		return nil, fmt.Errorf("failed to convert from unstructured PV: %w", err)
	}

	if pv.Spec.CSI.Driver != b.config["driver"] {
		return nil, fmt.Errorf("PV driver ('spec.csi.driver') doesn't match supported driver (%s)", b.config["driver"])
	}

	// get share access rule
	rule, err := b.getShareAccessRule(logWithFields, volumeID)
	if err != nil {
		return nil, err
	}

	pv.Spec.CSI.VolumeHandle = volumeID
	pv.Spec.CSI.VolumeAttributes["shareID"] = volumeID
	pv.Spec.CSI.VolumeAttributes["shareAccessID"] = rule.ID

	res, err := runtime.DefaultUnstructuredConverter.ToUnstructured(pv)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to unstructured PV: %w", err)
	}

	return &unstructured.Unstructured{Object: res}, nil
}

func (b *FSStore) getShareAccessRule(logWithFields *logrus.Entry, volumeID string) (*shares.AccessRight, error) {
	var rules interface{}
	var err error
	// deprecated API call
	if b.client.Microversion == minSupportedMicroversion {
		rules, err = shares.ListAccessRights(b.client, volumeID).Extract()
	} else {
		rules, err = shareaccessrules.List(b.client, volumeID).Extract()
	}
	if err != nil {
		logWithFields.Errorf("failed to list share %v access rules from manila", volumeID)
		return nil, fmt.Errorf("failed to list share %v access rules from manila: %w", volumeID, err)
	}

	switch rules := rules.(type) {
	case []shares.AccessRight:
		for _, rule := range rules {
			return &rule, nil
		}
	case []shareaccessrules.ShareAccess:
		for _, rule := range rules {
			return &shares.AccessRight{
				ID:          rule.ID,
				ShareID:     rule.ShareID,
				AccessKey:   rule.AccessKey,
				AccessLevel: rule.AccessLevel,
				AccessTo:    rule.AccessTo,
				AccessType:  rule.AccessType,
				State:       rule.State,
			}, nil
		}
	}

	logWithFields.Errorf("failed to find share %v access rules from manila", volumeID)
	return nil, fmt.Errorf("failed to find share %v access rules from manila: %w", volumeID, err)
}

func (b *FSStore) getManilaMicroversion() (string, error) {
	api, err := apiversions.Get(b.client, "v2").Extract()
	if err != nil {
		return "", err
	}
	return api.Version, nil
}

func (b *FSStore) waitForShareStatus(id string, statuses []string, secs int) (current *shares.Share, err error) {
	return current, gophercloud.WaitFor(secs, func() (bool, error) {
		current, err = shares.Get(b.client, id).Extract()
		if err != nil {
			return false, err
		}

		if utils.SliceContains(statuses, current.Status) {
			return true, nil
		}

		return false, nil
	})
}

func (b *FSStore) waitForSnapshotStatus(id string, statuses []string, secs int) (current *snapshots.Snapshot, err error) {
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
