package cinder

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/Lirt/velero-plugin-for-openstack/src/utils"
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/blockstorage/apiversions"
	"github.com/gophercloud/gophercloud/openstack/blockstorage/extensions/backups"
	"github.com/gophercloud/gophercloud/openstack/blockstorage/extensions/volumeactions"
	"github.com/gophercloud/gophercloud/openstack/blockstorage/v3/snapshots"
	"github.com/gophercloud/gophercloud/openstack/blockstorage/v3/volumes"
	"github.com/gophercloud/gophercloud/openstack/imageservice/v2/images"
	"github.com/sirupsen/logrus"
	velerovolumesnapshotter "github.com/vmware-tanzu/velero/pkg/plugin/velero/volumesnapshotter/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	defaultTimeout           = "5m"
	volumeBackupMicroversion = "3.47"
	volumeImageMicroversion  = "3.1"
	defaultDeleteDelay       = "10s"
)

var (
	// a list of supported snapshot methods
	supportedMethods = []string{
		"snapshot",
		"clone",
		"backup",
		"image",
	}
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
	// active backup statuses
	//   https://github.com/openstack/cinder/blob/master/api-ref/source/v3/ext-backups.inc#backups-backups
	backupStatuses = []string{
		"available",
	}
	// active image statuses
	//   https://github.com/openstack/glance/blob/master/api-ref/source/v2/images-images-v2.inc#images
	imageStatuses = []string{
		"active",
	}
	// a list of volume attributes to skip for image upload
	skipVolumeAttributes = []string{
		"direct_url",
		"boot_roles",
		"os_hash_algo",
		"os_hash_value",
		"checksum",
		"size",
		"container_format",
		"disk_format",
		"image_id",
		// these integer values have to be set separately
		"min_disk",
		"min_ram",
	}
)

// BlockStore is a plugin for containing state for the Cinder Block Storage
type BlockStore struct {
	client             *gophercloud.ServiceClient
	imgClient          *gophercloud.ServiceClient
	provider           *gophercloud.ProviderClient
	config             map[string]string
	volumeTimeout      int
	snapshotTimeout    int
	cloneTimeout       int
	backupTimeout      int
	imageTimeout       int
	ensureDeleted      bool
	ensureDeletedDelay int
	cascadeDelete      bool
	containerName      string
	log                logrus.FieldLogger
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

	// parse the snapshot method
	b.config["method"] = utils.GetConf(b.config, "method", "snapshot")
	if !utils.SliceContains(supportedMethods, b.config["method"]) {
		return fmt.Errorf("unsupported %q snapshot method, supported methods: %q", b.config["method"], supportedMethods)
	}

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
	b.cloneTimeout, err = utils.DurationToSeconds(utils.GetConf(b.config, "cloneTimeout", defaultTimeout))
	if err != nil {
		return fmt.Errorf("cannot parse time from cloneTimeout config variable: %w", err)
	}
	b.backupTimeout, err = utils.DurationToSeconds(utils.GetConf(b.config, "backupTimeout", defaultTimeout))
	if err != nil {
		return fmt.Errorf("cannot parse time from backupTimeout config variable: %w", err)
	}
	b.imageTimeout, err = utils.DurationToSeconds(utils.GetConf(b.config, "imageTimeout", defaultTimeout))
	if err != nil {
		return fmt.Errorf("cannot parse time from imageTimeout config variable: %w", err)
	}
	// parse options
	b.ensureDeleted, err = strconv.ParseBool(utils.GetConf(b.config, "ensureDeleted", "false"))
	if err != nil {
		return fmt.Errorf("cannot parse ensureDeleted config variable: %w", err)
	}
	b.ensureDeletedDelay, err = utils.DurationToSeconds(utils.GetConf(b.config, "ensureDeletedDelay", defaultDeleteDelay))
	if err != nil {
		return fmt.Errorf("cannot parse time from ensureDeletedDelay config variable: %w", err)
	}
	b.cascadeDelete, err = strconv.ParseBool(utils.GetConf(b.config, "cascadeDelete", "false"))
	if err != nil {
		return fmt.Errorf("cannot parse cascadeDelete config variable: %w", err)
	}

	// load optional containerName
	b.containerName = utils.GetConf(b.config, "containerName", "")

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
				region = ""
			}
		}
		b.client, err = openstack.NewBlockStorageV3(b.provider, gophercloud.EndpointOpts{
			Region: region,
		})
		if err != nil {
			return fmt.Errorf("failed to create cinder storage client: %w", err)
		}

		logWithFields := b.log.WithFields(logrus.Fields{
			"endpoint": b.client.Endpoint,
			"region":   region,
		})

		// set minimum supported Cinder microversion for backups or images
		switch b.config["method"] {
		case "backup":
			err = b.setCinderMicroversion(volumeBackupMicroversion)
			if err != nil {
				return err
			}
			logWithFields.Infof("Setting the supported %v microversion", b.client.Microversion)
		case "image":
			err = b.setCinderMicroversion(volumeImageMicroversion)
			if err != nil {
				return err
			}
			logWithFields.Infof("Setting the supported %v microversion", b.client.Microversion)

			b.imgClient, err = openstack.NewImageServiceV2(b.provider, gophercloud.EndpointOpts{
				Region: region,
			})
			if err != nil {
				return fmt.Errorf("failed to create glance image client: %w", err)
			}

			logWithFields.Info("Successfully created image service client")
		}

		logWithFields.Info("Successfully created block storage service client")
	}

	return nil
}

// CreateVolumeFromSnapshot creates a new volume in the specified
// availability zone, initialized from the provided snapshot and with the specified type.
// IOPS is ignored as it is not used in Cinder.
func (b *BlockStore) CreateVolumeFromSnapshot(snapshotID, volumeType, volumeAZ string, iops *int64) (string, error) {
	switch b.config["method"] {
	case "clone":
		return b.createVolumeFromClone(snapshotID, volumeType, volumeAZ)
	case "backup":
		return b.createVolumeFromBackup(snapshotID, volumeType, volumeAZ)
	case "image":
		return b.createVolumeFromImage(snapshotID, volumeType, volumeAZ)
	}

	return b.createVolumeFromSnapshot(snapshotID, volumeType, volumeAZ)
}

func (b *BlockStore) createVolumeFromSnapshot(snapshotID, volumeType, volumeAZ string) (string, error) {
	logWithFields := b.log.WithFields(logrus.Fields{
		"snapshotID":      snapshotID,
		"volumeType":      volumeType,
		"volumeAZ":        volumeAZ,
		"snapshotTimeout": b.snapshotTimeout,
		"volumeTimeout":   b.volumeTimeout,
		"method":          b.config["method"],
	})
	logWithFields.Info("BlockStore.CreateVolumeFromSnapshot called")

	volumeName := fmt.Sprintf("%s.backup.%s", snapshotID, strconv.FormatUint(utils.Rand.Uint64(), 10))
	// Make sure snapshot is in ready state
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
		return volume.ID, fmt.Errorf("volume %v didn't get into 'available' state within the time limit: %w", volume.ID, err)
	}

	logWithFields.WithFields(logrus.Fields{
		"volumeID": volume.ID,
	}).Info("Backup volume was created")
	return volume.ID, nil
}

func (b *BlockStore) createVolumeFromClone(cloneID, volumeType, volumeAZ string) (string, error) {
	logWithFields := b.log.WithFields(logrus.Fields{
		"cloneID":       cloneID,
		"volumeType":    volumeType,
		"volumeAZ":      volumeAZ,
		"cloneTimeout":  b.cloneTimeout,
		"volumeTimeout": b.volumeTimeout,
		"method":        b.config["method"],
	})
	logWithFields.Info("BlockStore.CreateVolumeFromSnapshot called")

	volumeName := fmt.Sprintf("%s.backup.%s", cloneID, strconv.FormatUint(utils.Rand.Uint64(), 10))
	volumeDesc := "Velero backup from volume clone"
	volumeID, err := b.cloneVolume(logWithFields, cloneID, volumeName, volumeDesc, volumeAZ, nil)
	if err != nil {
		return volumeID, err
	}

	logWithFields.WithFields(logrus.Fields{
		"volumeID": volumeID,
	}).Info("Backup volume was created")
	return volumeID, nil
}

func (b *BlockStore) createVolumeFromBackup(backupID, volumeType, volumeAZ string) (string, error) {
	logWithFields := b.log.WithFields(logrus.Fields{
		"backupID":      backupID,
		"volumeType":    volumeType,
		"volumeAZ":      volumeAZ,
		"backupTimeout": b.backupTimeout,
		"volumeTimeout": b.volumeTimeout,
		"method":        b.config["method"],
	})
	logWithFields.Info("BlockStore.CreateVolumeFromSnapshot called")

	volumeName := fmt.Sprintf("%s.backup.%s", backupID, strconv.FormatUint(utils.Rand.Uint64(), 10))
	// Make sure backup is in ready state
	logWithFields.Info("Waiting for backup to be in 'available' state")

	backup, err := b.waitForBackupStatus(backupID, backupStatuses, b.backupTimeout)
	if err != nil {
		logWithFields.Error("backup didn't get into 'available' state within the time limit")
		return "", fmt.Errorf("backup %v didn't get into 'available' state within the time limit: %w", backupID, err)
	}
	logWithFields.Info("Backup is in 'available' state")

	// Create Cinder Volume from backup (backup)
	logWithFields.Info("Starting to create volume from backup")
	opts := volumes.CreateOpts{
		Description:      "Velero backup from backup",
		Name:             volumeName,
		VolumeType:       volumeType,
		AvailabilityZone: volumeAZ,
		BackupID:         backupID,
	}
	if backup.Metadata != nil {
		opts.Metadata = *backup.Metadata
	}

	volume, err := volumes.Create(b.client, opts).Extract()
	if err != nil {
		logWithFields.Error("failed to create volume from backup")
		return "", fmt.Errorf("failed to create volume %v from backup %v: %w", volumeName, backupID, err)
	}

	_, err = b.waitForVolumeStatus(volume.ID, volumeStatuses, b.volumeTimeout)
	if err != nil {
		logWithFields.Error("volume didn't get into 'available' state within the time limit")
		return volume.ID, fmt.Errorf("volume %v didn't get into 'available' state within the time limit: %w", volume.ID, err)
	}

	logWithFields.WithFields(logrus.Fields{
		"volumeID": volume.ID,
	}).Info("Backup volume was created")
	return volume.ID, nil
}

func (b *BlockStore) createVolumeFromImage(imageID, volumeType, volumeAZ string) (string, error) {
	logWithFields := b.log.WithFields(logrus.Fields{
		"imageID":       imageID,
		"volumeType":    volumeType,
		"volumeAZ":      volumeAZ,
		"imageTimeout":  b.imageTimeout,
		"volumeTimeout": b.volumeTimeout,
		"method":        b.config["method"],
	})
	logWithFields.Info("BlockStore.CreateVolumeFromSnapshot called")

	volumeName := fmt.Sprintf("%s.image.%s", imageID, strconv.FormatUint(utils.Rand.Uint64(), 10))
	// Make sure image is in ready state
	logWithFields.Info("Waiting for image to be in 'available' state")

	_, err := b.waitForImageStatus(imageID, imageStatuses, b.imageTimeout)
	if err != nil {
		logWithFields.Error("image didn't get into 'active' state within the time limit")
		return "", fmt.Errorf("image %v didn't get into 'active' state within the time limit: %w", imageID, err)
	}
	logWithFields.Info("Image is in 'active' state")

	// Create Cinder Volume from image (image)
	logWithFields.Info("Starting to create volume from image")
	opts := volumes.CreateOpts{
		Description:      "Velero backup from image",
		Name:             volumeName,
		VolumeType:       volumeType,
		AvailabilityZone: volumeAZ,
		ImageID:          imageID,
		// TODO: add Metadata support
	}

	volume, err := volumes.Create(b.client, opts).Extract()
	if err != nil {
		logWithFields.Error("failed to create volume from image")
		return "", fmt.Errorf("failed to create volume %v from image %v: %w", volumeName, imageID, err)
	}

	_, err = b.waitForVolumeStatus(volume.ID, volumeStatuses, b.volumeTimeout)
	if err != nil {
		logWithFields.Error("volume didn't get into 'available' state within the time limit")
		return volume.ID, fmt.Errorf("volume %v didn't get into 'available' state within the time limit: %w", volume.ID, err)
	}

	logWithFields.WithFields(logrus.Fields{
		"volumeID": volume.ID,
	}).Info("Backup volume was created")
	return volume.ID, nil
}

func (b *BlockStore) cloneVolume(logWithFields *logrus.Entry, volumeID, volumeName, volumeDesc, volumeAZ string, tags map[string]string) (string, error) {
	// Make sure source volume clone is in ready state
	logWithFields.Info("Waiting for source volume clone to be in 'available' state")

	originVolume, err := b.waitForVolumeStatus(volumeID, volumeStatuses, b.volumeTimeout)
	if err != nil {
		logWithFields.Error("source volume clone didn't get into 'available' state within the time limit")
		return "", fmt.Errorf("source volume clone %v didn't get into 'available' state within the time limit: %w", volumeID, err)
	}
	logWithFields.Info("Source volume is in 'available' state")

	// Create Cinder Volume from volume (backup)
	logWithFields.Info("Starting to create volume from clone")
	opts := volumes.CreateOpts{
		Name:             volumeName,
		Description:      volumeDesc,
		VolumeType:       originVolume.VolumeType,
		AvailabilityZone: volumeAZ,
		SourceVolID:      volumeID,
		Metadata:         utils.Merge(originVolume.Metadata, tags),
	}

	volume, err := volumes.Create(b.client, opts).Extract()
	if err != nil {
		logWithFields.Error("failed to create volume from volume clone")
		return "", fmt.Errorf("failed to create volume %v from volume clone %v: %w", volumeName, volumeID, err)
	}

	_, err = b.waitForVolumeStatus(volume.ID, volumeStatuses, b.volumeTimeout)
	if err != nil {
		logWithFields.Error("volume didn't get into 'available' state within the time limit")
		return volume.ID, fmt.Errorf("volume %v didn't get into 'available' state within the time limit: %w", volume.ID, err)
	}

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
	switch b.config["method"] {
	case "clone":
		return b.createClone(volumeID, volumeAZ, tags)
	case "backup":
		return b.createBackup(volumeID, volumeAZ, tags)
	case "image":
		return b.createImage(volumeID, volumeAZ, tags)
	}

	return b.createSnapshot(volumeID, volumeAZ, tags)
}

func (b *BlockStore) createSnapshot(volumeID, volumeAZ string, tags map[string]string) (string, error) {
	snapshotName := fmt.Sprintf("%s.snap.%s", volumeID, strconv.FormatUint(utils.Rand.Uint64(), 10))
	logWithFields := b.log.WithFields(logrus.Fields{
		"snapshotName":    snapshotName,
		"volumeID":        volumeID,
		"volumeAZ":        volumeAZ,
		"tags":            tags,
		"snapshotTimeout": b.snapshotTimeout,
		"volumeTimeout":   b.volumeTimeout,
		"method":          b.config["method"],
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
		return snapshot.ID, fmt.Errorf("snapshot %v didn't get into 'available' state within the time limit: %w", snapshot.ID, err)
	}
	logWithFields.Info("Snapshot is in 'available' state")

	logWithFields.WithFields(logrus.Fields{
		"snapshotID": snapshot.ID,
	}).Info("Snapshot finished successfuly")
	return snapshot.ID, nil
}

func (b *BlockStore) createClone(volumeID, volumeAZ string, tags map[string]string) (string, error) {
	cloneName := fmt.Sprintf("%s.clone.%s", volumeID, strconv.FormatUint(utils.Rand.Uint64(), 10))
	logWithFields := b.log.WithFields(logrus.Fields{
		"cloneName":    cloneName,
		"volumeID":     volumeID,
		"volumeAZ":     volumeAZ,
		"tags":         tags,
		"cloneTimeout": b.cloneTimeout,
		"method":       b.config["method"],
	})
	logWithFields.Info("BlockStore.CreateSnapshot called")

	cloneDesc := "Velero volume clone"
	cloneID, err := b.cloneVolume(logWithFields, volumeID, cloneName, cloneDesc, volumeAZ, tags)
	if err != nil {
		return cloneID, err
	}

	logWithFields.WithFields(logrus.Fields{
		"cloneID": cloneID,
	}).Info("Volume clone finished successfuly")
	return cloneID, nil
}

func (b *BlockStore) createBackup(volumeID, volumeAZ string, tags map[string]string) (string, error) {
	backupName := fmt.Sprintf("%s.backup.%s", volumeID, strconv.FormatUint(utils.Rand.Uint64(), 10))
	logWithFields := b.log.WithFields(logrus.Fields{
		"backupName":    backupName,
		"volumeID":      volumeID,
		"volumeAZ":      volumeAZ,
		"tags":          tags,
		"backupTimeout": b.backupTimeout,
		"method":        b.config["method"],
	})
	logWithFields.Info("BlockStore.CreateSnapshot called")

	originVolume, err := volumes.Get(b.client, volumeID).Extract()
	if err != nil {
		logWithFields.Error("failed to get volume from cinder")
		return "", fmt.Errorf("failed to get volume %v from cinder: %w", volumeID, err)
	}

	opts := &backups.CreateOpts{
		Name:        backupName,
		VolumeID:    volumeID,
		Description: "Velero volume backup",
		Container:   backupName,
		Metadata:    utils.Merge(originVolume.Metadata, tags),
		Force:       true,
	}

	// Override container if one was passed by the user
	if b.containerName != "" {
		opts.Container = b.containerName
	}

	backup, err := backups.Create(b.client, opts).Extract()
	if err != nil {
		logWithFields.Error("failed to create backup from volume")
		return "", fmt.Errorf("failed to create backup %v from volume %v: %w", backupName, volumeID, err)
	}

	_, err = b.waitForBackupStatus(backup.ID, backupStatuses, b.backupTimeout)
	if err != nil {
		logWithFields.Error("backup didn't get into 'available' state within the time limit")
		return backup.ID, fmt.Errorf("backup %v didn't get into 'available' state within the time limit: %w", backup.ID, err)
	}
	logWithFields.Info("Volume backup is in 'available' state")

	logWithFields.WithFields(logrus.Fields{
		"backupID": backup.ID,
	}).Info("Volume backup finished successfuly")
	return backup.ID, nil
}

func (b *BlockStore) createImage(volumeID, volumeAZ string, tags map[string]string) (string, error) {
	imageName := fmt.Sprintf("%s.image.%s", volumeID, strconv.FormatUint(utils.Rand.Uint64(), 10))
	logWithFields := b.log.WithFields(logrus.Fields{
		"imageName":    imageName,
		"volumeID":     volumeID,
		"volumeAZ":     volumeAZ,
		"tags":         tags,
		"imageTimeout": b.imageTimeout,
		"method":       b.config["method"],
	})
	logWithFields.Info("BlockStore.CreateSnapshot called")

	originVolume, err := volumes.Get(b.client, volumeID).Extract()
	if err != nil {
		logWithFields.Error("failed to get volume from cinder")
		return "", fmt.Errorf("failed to get volume %v from cinder: %w", volumeID, err)
	}

	opts := &volumeactions.UploadImageOpts{
		ImageName: imageName,
		// Description: "Velero volume image",
		ContainerFormat: originVolume.VolumeImageMetadata["container_format"],
		DiskFormat:      originVolume.VolumeImageMetadata["disk_format"],
		Visibility:      string(images.ImageVisibilityPrivate),
		Force:           true,
		// TODO: add Metadata support
	}
	image, err := volumeactions.UploadImage(b.client, volumeID, opts).Extract()
	if err != nil {
		logWithFields.Error("failed to create image from volume")
		return "", fmt.Errorf("failed to create image %v from volume %v: %w", imageName, volumeID, err)
	}

	_, err = b.waitForImageStatus(image.ImageID, imageStatuses, b.imageTimeout)
	if err != nil {
		logWithFields.Error("image didn't get into 'active' state within the time limit")
		return image.ImageID, fmt.Errorf("image %v didn't get into 'active' state within the time limit: %w", image.ImageID, err)
	}
	logWithFields.Info("Volume image is in 'active' state")

	updateProperties := expandVolumeProperties(logWithFields, originVolume)
	_, err = images.Update(b.imgClient, image.ImageID, updateProperties).Extract()
	if err != nil {
		logWithFields.Error("failed to update image properties")
		return image.ImageID, fmt.Errorf("failed to update image properties: %w", err)
	}

	logWithFields.WithFields(logrus.Fields{
		"imageID": image.ImageID,
	}).Info("Volume image finished successfuly")
	return image.ImageID, nil
}

// DeleteSnapshot deletes the specified volume snapshot.
func (b *BlockStore) DeleteSnapshot(snapshotID string) error {
	switch b.config["method"] {
	case "clone":
		return b.deleteClone(snapshotID)
	case "backup":
		return b.deleteBackup(snapshotID)
	case "image":
		return b.deleteImage(snapshotID)
	}

	return b.deleteSnapshot(snapshotID)
}

func (b *BlockStore) deleteSnapshot(snapshotID string) error {
	logWithFields := b.log.WithFields(logrus.Fields{
		"snapshotID": snapshotID,
		"method":     b.config["method"],
	})
	logWithFields.Info("BlockStore.DeleteSnapshot called")

	// Delete snapshot from Cinder
	if b.ensureDeleted {
		logWithFields.Infof("waiting for a %s snapshot to be deleted", snapshotID)
		return b.ensureSnapshotDeleted(logWithFields, snapshotID, b.snapshotTimeout)
	}

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

// deleteSnapshots removes all the volume snapshots
func (b *BlockStore) deleteSnapshots(logWithFields *logrus.Entry, volumeID string) error {
	listOpts := snapshots.ListOpts{
		VolumeID: volumeID,
	}
	pages, err := snapshots.List(b.client, listOpts).AllPages()
	if err != nil {
		return fmt.Errorf("failed to list %s volume snapshots: %w", volumeID, err)
	}
	allSnapshots, err := snapshots.ExtractSnapshots(pages)
	if err != nil {
		return fmt.Errorf("failed to extract %s volume snapshots: %w", volumeID, err)
	}

	wg := sync.WaitGroup{}
	errs := make(chan error, len(allSnapshots))
	deleteSnapshot := func(snapshotID string) {
		logWithFields.Infof("deleting the %s snapshot", snapshotID)
		err := b.ensureSnapshotDeleted(logWithFields, snapshotID, b.snapshotTimeout)
		if err != nil {
			logWithFields.Errorf("failed to delete %s volume snapshot: %v", snapshotID, err)
			errs <- fmt.Errorf("failed to delete %s volume snapshot: %w", snapshotID, err)
		}
		wg.Done()
	}

	for _, snapshot := range allSnapshots {
		wg.Add(1)
		go deleteSnapshot(snapshot.ID)
	}

	wg.Wait()
	close(errs)

	for e := range errs {
		err = errors.Join(err, e)
	}

	return err
}

func (b *BlockStore) deleteClone(cloneID string) error {
	logWithFields := b.log.WithFields(logrus.Fields{
		"cloneID": cloneID,
		"method":  b.config["method"],
	})
	logWithFields.Info("BlockStore.DeleteSnapshot called")

	// cascade deletion of the volume dependent resources
	if cloneID != "" && b.cascadeDelete {
		err := b.deleteSnapshots(logWithFields, cloneID)
		if err != nil {
			return fmt.Errorf("failed to delete %s volume snapshots: %w", cloneID, err)
		}
	}

	// Delete volume clone from Cinder
	if b.ensureDeleted {
		logWithFields.Infof("waiting for a %s clone volume to be deleted", cloneID)
		return b.ensureVolumeDeleted(logWithFields, cloneID, b.cloneTimeout)
	}

	err := volumes.Delete(b.client, cloneID, nil).ExtractErr()
	if err != nil {
		if _, ok := err.(gophercloud.ErrDefault404); ok {
			logWithFields.Info("volume clone is already deleted")
			return nil
		}
		logWithFields.Error("failed to delete volume clone")
		return fmt.Errorf("failed to delete volume clone %v: %w", cloneID, err)
	}

	return nil
}

func (b *BlockStore) deleteBackup(backupID string) error {
	logWithFields := b.log.WithFields(logrus.Fields{
		"backupID": backupID,
		"method":   b.config["method"],
	})
	logWithFields.Info("BlockStore.DeleteSnapshot called")

	// Delete volume backup from Cinder
	if b.ensureDeleted {
		logWithFields.Infof("waiting for a %s volume backup deleted", backupID)
		return b.ensureBackupDeleted(logWithFields, backupID, b.backupTimeout)
	}

	err := backups.Delete(b.client, backupID).ExtractErr()
	if err != nil {
		if _, ok := err.(gophercloud.ErrDefault404); ok {
			logWithFields.Info("volume backup is already deleted")
			return nil
		}
		logWithFields.Error("failed to delete volume backup")
		return fmt.Errorf("failed to delete volume backup %v: %w", backupID, err)
	}

	return nil
}

func (b *BlockStore) deleteImage(imageID string) error {
	logWithFields := b.log.WithFields(logrus.Fields{
		"imageID": imageID,
		"method":  b.config["method"],
	})
	logWithFields.Info("BlockStore.DeleteSnapshot called")

	// Delete volume image from Glance
	err := images.Delete(b.imgClient, imageID).ExtractErr()
	if err != nil {
		if _, ok := err.(gophercloud.ErrDefault404); ok {
			logWithFields.Info("volume image is already deleted")
			return nil
		}
		logWithFields.Error("failed to delete volume image")
		return fmt.Errorf("failed to delete volume image %v: %w", imageID, err)
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

func (b *BlockStore) getCinderMicroversion() (string, error) {
	allVersions, err := apiversions.List(b.client).AllPages()
	if err != nil {
		return "", err
	}
	api, err := apiversions.ExtractAPIVersion(allVersions, "v3.0")
	if err != nil {
		return "", err
	}
	return api.Version, nil
}

func (b *BlockStore) setCinderMicroversion(version string) error {
	mv, err := b.getCinderMicroversion()
	if err != nil {
		return fmt.Errorf("failed to obtain supported Cinder microversions: %v", err)
	}
	ok, err := utils.CompareMicroversions("lte", version, mv)
	if err != nil {
		return fmt.Errorf("failed to compare supported Cinder microversions: %v", err)
	}
	if !ok {
		return fmt.Errorf("the %v Cinder microversion doesn't support %ss", mv, b.config["method"])
	}

	b.client.Microversion = version

	return nil
}

func (b *BlockStore) waitForVolumeStatus(id string, statuses []string, secs int) (current *volumes.Volume, err error) {
	return current, utils.WaitForStatus(statuses, secs, func() (string, error) {
		current, err = volumes.Get(b.client, id).Extract()
		if err != nil {
			return "", err
		}
		return current.Status, nil
	})
}

func (b *BlockStore) waitForSnapshotStatus(id string, statuses []string, secs int) (current *snapshots.Snapshot, err error) {
	return current, utils.WaitForStatus(statuses, secs, func() (string, error) {
		current, err = snapshots.Get(b.client, id).Extract()
		if err != nil {
			return "", err
		}
		return current.Status, nil
	})
}

func (b *BlockStore) waitForBackupStatus(id string, statuses []string, secs int) (current *backups.Backup, err error) {
	return current, utils.WaitForStatus(statuses, secs, func() (string, error) {
		current, err = backups.Get(b.client, id).Extract()
		if err != nil {
			return "", err
		}
		return current.Status, nil
	})
}

func (b *BlockStore) waitForImageStatus(id string, statuses []string, secs int) (current *images.Image, err error) {
	return current, utils.WaitForStatus(statuses, secs, func() (string, error) {
		current, err = images.Get(b.imgClient, id).Extract()
		if err != nil {
			return "", err
		}
		return string(current.Status), nil
	})
}

func (b *BlockStore) ensureVolumeDeleted(logWithFields *logrus.Entry, id string, secs int) error {
	deleteFunc := func() error {
		err := volumes.Delete(b.client, id, nil).ExtractErr()
		if err != nil {
			logWithFields.Infof("failed to delete a %s volume: %v", id, err)
		}
		return err
	}
	checkFunc := func() error {
		_, err := b.waitForVolumeStatus(id, []string{"deleted"}, secs)
		if err != nil {
			logWithFields.Infof("failed to wait for a %s volume status: %v", id, err)
		}
		return err
	}
	resetFunc := func() error {
		logWithFields.Infof("resetting a %s volume status and trying again", id)
		opts := &volumeactions.ResetStatusOpts{
			Status: "error",
		}
		err := volumeactions.ResetStatus(b.client, id, opts).ExtractErr()
		if err != nil {
			logWithFields.Infof("failed to reset a %s volume status: %v", id, err)
		}
		return err
	}

	return utils.EnsureDeleted(deleteFunc, checkFunc, resetFunc, secs, b.ensureDeletedDelay)
}

func (b *BlockStore) ensureSnapshotDeleted(logWithFields *logrus.Entry, id string, secs int) error {
	deleteFunc := func() error {
		err := snapshots.Delete(b.client, id).ExtractErr()
		if err != nil {
			logWithFields.Infof("failed to delete a %s snapshot: %v", id, err)
		}
		return err
	}
	checkFunc := func() error {
		_, err := b.waitForSnapshotStatus(id, []string{"deleted"}, secs)
		if err != nil {
			logWithFields.Infof("failed to wait for a %s snapshot status: %v", id, err)
		}
		return err
	}
	resetFunc := func() error {
		logWithFields.Infof("resetting a %s snapshot status and trying again", id)
		opts := &snapshots.ResetStatusOpts{
			Status: "error",
		}
		err := snapshots.ResetStatus(b.client, id, opts).ExtractErr()
		if err != nil {
			logWithFields.Infof("failed to reset a %s snapshot status: %v", id, err)
		}
		return err
	}

	return utils.EnsureDeleted(deleteFunc, checkFunc, resetFunc, secs, b.ensureDeletedDelay)
}

func (b *BlockStore) ensureBackupDeleted(logWithFields *logrus.Entry, id string, secs int) error {
	deleteFunc := func() error {
		err := backups.Delete(b.client, id).ExtractErr()
		if err != nil {
			logWithFields.Infof("failed to delete a %s backup: %v", id, err)
		}
		return err
	}
	checkFunc := func() error {
		_, err := b.waitForBackupStatus(id, []string{"deleted"}, secs)
		if err != nil {
			logWithFields.Infof("failed to wait for a %s backup status: %v", id, err)
		}
		return err
	}
	resetFunc := func() error {
		logWithFields.Infof("resetting a %s backup status and trying again", id)
		opts := &backups.ResetStatusOpts{
			Status: "error",
		}
		err := backups.ResetStatus(b.client, id, opts).ExtractErr()
		if err != nil {
			logWithFields.Infof("failed to reset a %s backup status: %v", id, err)
		}
		return err
	}

	return utils.EnsureDeleted(deleteFunc, checkFunc, resetFunc, secs, b.ensureDeletedDelay)
}

func expandVolumeProperties(log logrus.FieldLogger, volume *volumes.Volume) images.UpdateOpts {
	// set min_disk and min_ram from a source volume
	imgAttrUpdateOpts := images.UpdateOpts{
		images.ReplaceImageMinDisk{NewMinDisk: volume.Size},
	}
	if s, ok := volume.VolumeImageMetadata["min_ram"]; ok {
		if minRam, err := strconv.Atoi(s); err == nil {
			imgAttrUpdateOpts = append(imgAttrUpdateOpts, images.ReplaceImageMinRam{NewMinRam: minRam})
		} else {
			log.Warningf("Cannot convert %q to integer: %s", s, err)
		}
	}
	for key, value := range volume.VolumeImageMetadata {
		if utils.SliceContains(skipVolumeAttributes, key) || value == "" {
			continue
		}
		imgAttrUpdateOpts = append(imgAttrUpdateOpts, images.UpdateImageProperty{
			Op:    images.AddOp,
			Name:  key,
			Value: value,
		})
	}
	return imgAttrUpdateOpts
}
