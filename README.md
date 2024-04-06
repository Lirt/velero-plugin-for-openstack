# Velero Plugin for OpenStack

OpenStack Cinder, Manila and Swift plugin for [velero](https://github.com/vmware-tanzu/velero/) backups.

This plugin is [included as community supported plugin by Velero organization](https://velero.io/plugins/).

## Table of Contents

- [Velero Plugin for OpenStack](#velero-plugin-for-openstack)
  - [Compatibility](#compatibility)
  - [OpenStack Authentication Configuration](#openstack-authentication-configuration)
  - [Installation](#installation)
    - [Swift Container Setup](#swift-container-setup)
  - [Volume Backups](#volume-backups)
    - [Backup Methods](#backup-methods)
    - [Consistency and Durability](#consistency-and-durability)
    - [Native VolumeSnapshots](#native-volumesnapshots)
    - [Restic](#restic)
  - [Known Issues](#known-issues)
  - [Test & Build](#test--build)
  - [Development](#development)

## Compatibility

Below is a matrix of plugin versions and Velero versions for which the compatibility is tested and guaranteed.

| Plugin Version | Velero Version |
| :------------- | :------------- |
| v0.6.x         | 1.9.x, 1.10.x 1.11.x |
| v0.5.x         | v1.4.x, v1.5.x, v1.6.x, v1.7.x, v1.8.x, 1.9.x, 1.10.x 1.11.x |

## OpenStack Authentication Configuration

The order of authentication methods is following:
1. Authentication using environment variables takes precedence (including [Application Credentials](https://docs.openstack.org/keystone/queens/user/application_credentials.html#using-application-credentials)). You **must not** set env. variable `OS_CLOUD` when you want to authenticate using env. variables because authenticator will try to look for `clouds.y(a)ml` file and use it.
1. Authentication using files is second option. Note: you will also need to set `OS_CLOUD` environment variable to tell which cloud from `clouds.y(a)ml` will be used:
  1. If `OS_CLIENT_CONFIG_FILE` env. variable is specified, code will authenticate using this file.
  1. Look for file `clouds.y(a)ml` in current directory.
  1. Look for file in `~/.config/openstack/clouds.y(a)ml`.
  1. Look for file in `/etc/openstack/clouds.y(a)ml`.

For authentication using application credentials you first need to create credentials using openstack CLI command such as `openstack application credential create <NAME>`.

For more information about how to configure authentication, see one of following documents:
1. [Authentication using Environment Variables](docs/authentication-env.md)
1. [Authentication using Files](docs/authentication-file.md)

Both authentication options also allow you to authenticate against multiple OpenStack Clouds at the same time. The way you can leverage this functionality is scenario where you want to store backups in 2 different locations. This scenario doesn't apply for Volume Snapshots as they always need to be created in the same cloud and region as where your PVCs are created!

Example of multi-cloud BSL setup:
```yaml
---
apiVersion: velero.io/v1
kind: BackupStorageLocation
metadata:
  name: my-backup-in-cloud1
  namespace: velero
spec:
  accessMode: ReadWrite
  config:
    cloud: cloud1
    # optional region
    region: fra1
  default: false
  objectStorage:
    bucket: velero-backup-cloud1
  provider: community.openstack.org/openstack
---
apiVersion: velero.io/v1
kind: BackupStorageLocation
metadata:
  name: my-backup-in-cloud2
  namespace: velero
spec:
  accessMode: ReadWrite
  config:
    cloud: cloud2
    # optional region
    region: lon
  default: false
  objectStorage:
    bucket: velero-backup-cloud2
  provider: community.openstack.org/openstack
```

## Installation

There are 2 options how to install this plugin. Each method has a documentation subpage:
1. [Installation using CLI](docs/installation-using-cli.md)
1. [Installation using Helm](docs/installation-using-helm.md)

### Swift Container Setup

Swift container must have [Temporary URL Key](https://docs.openstack.org/swift/latest/api/temporary_url_middleware.html) configured to make it possible to download Velero backups. In your Swift project you can execute following command to configure it:

```bash
SWIFT_TMP_URL_KEY=$(dd if=/dev/urandom | LC_ALL=C tr -dc A-Za-z0-9 | head -c 40)
swift post -m "Temp-URL-Key:${SWIFT_TMP_URL_KEY}"
```

Or per container Temporary URL key:

```bash
SWIFT_TMP_URL_KEY=$(dd if=/dev/urandom | LC_ALL=C tr -dc A-Za-z0-9 | head -c 40)
swift post -m "Temp-URL-Key:${SWIFT_TMP_URL_KEY}" my-container
```

> **Note:** If the Swift account ID is overridden (for example, if the current authentication project scope does not correspond to the destination container project ID), you must set the corresponding valid `OS_SWIFT_TEMP_URL_KEY` environment variable.

## Volume Backups

### Backup Methods

Plugin supports multiple methods of creating a backup.

Cinder backup methods:
- **Snapshot** - Create a snapshot using Cinder.
- **Clone** - Clone a volume using Cinder.
- **Backup** - Create a backup using Cinder backup functionality (known in CLI as `cinder backup create`) - see [docs](https://docs.openstack.org/cinder/latest/admin/volume-backups.html).
- **Image** - Upload a volume into Glance image service (requires `enable_force_upload` Cinder option enabled on the server side).

Manila backup methods:
- **Snapshot** - Create a snapshot using Manila.
- **Clone** - Create a snapshot using Manila, but immediatelly create a volume from this snapshot and afterwards cleanup original snapshot.

### Consistency and Durability

Please note two facts regarding volume backups:
1. The snapshots are done using flag `--force`. The reason is that volumes in state `in-use` cannot be snapshotted without it (they would need to be detached in advance). In some cases this can make snapshot contents inconsistent!
2. Durability of backups in the Cinder or Manila backend depends on backup method that you will use. In most cases for proper availability, the snapshot needs to be backed up to off-site storage (in order to survive real datacenter incident). Please consult if chosen backup method and your Cinder or Manila backend setup will result in durable backups with your cloud provider.

### Native VolumeSnapshots

Alternative Kubernetes native solution (GA since 1.20) for volume snapshots are [VolumeSnapshots](https://kubernetes.io/docs/concepts/storage/volume-snapshots/) using [snapshot-controller](https://kubernetes-csi.github.io/docs/snapshot-controller.html).

### Restic

Volume backups with Velero can also be done using [Restic and Kopia](https://velero.io/docs/main/file-system-backup/). Please understand that this repository does not provide any functionality for restic and kopia and their implementation is done purely in Velero code!

There is a common similarity that `restic` can use OpenStack Swift as object storage for backups. Restic way of authentication and implementation is however very different from this repository and it means that some ways of authentication that work here will not work with restic. Please refer to [official restic documentation](https://restic.readthedocs.io/en/latest/030_preparing_a_new_repo.html#openstack-swift) to understand how are you supposed to configure authentication variables with restic.

Recommended way of using this plugin with restic is to use authentication with environment variables and only for 1 cloud and 1 BackupStorageLocation. In the BSL you need to configure `config.resticRepoPrefix: swift:<CONTAINER_NAME>:/<PATH>` - for example `config.resticRepoPrefix: swift:my-awesome-container:/restic`.

## Known Issues

- [Incompatibility with Cinder version 13.0.0 (Rocky)](https://github.com/Lirt/velero-plugin-for-openstack/issues/20)

## Test & Build

```bash
# test and build code
go test -v -count 1 ./...
go mod tidy
go build

# Build and push image for linux amd64, arm64, arm
docker buildx build \
              --file docker/Dockerfile \
              --platform linux/amd64,linux/arm/v6,linux/arm/v7,linux/arm64 \
              --tag lirt/velero-plugin-for-openstack:v0.6.0 \
              --build-arg VERSION=v0.6.0 \
              --build-arg GIT_SHA=somesha \
              --no-cache \
              --push \
              .
```

## Development

The plugin interface is built based on the [official Velero plugin example](https://github.com/vmware-tanzu/velero-plugin-example).
