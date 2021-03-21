# Velero Plugin for Openstack

Openstack Cinder and Swift plugin for [velero](https://github.com/vmware-tanzu/velero/) backups.

## Table of Contents

- [Velero Plugin for Openstack](#velero-plugin-for-openstack)
  - [Compatibility](#compatibility)
  - [Configuration](#configuration)
    - [Install Using Velero CLI](#install-using-velero-cli)
    - [Install Using Helm Chart](#install-using-helm-chart)
  - [Volume Backups](#volume-backups)
  - [Build](#build)
  - [Test](#test)
  - [Development](#development)

## Compatibility

Below is a listing of plugin versions and respective Velero versions for which the compatibility is tested and guaranteed.

| Plugin Version | Velero Version |
| :------------- | :------------- |
| v0.2.x         | v1.4.x, v1.5.x |
| v0.1.x         | v1.4.x, v1.5.x |

## Configuration

Configure velero container with your Openstack authentication environment variables:

```bash
# Keystone v2.0
export OS_AUTH_URL=<AUTH_URL /v2.0>
export OS_USERNAME=<USERNAME>
export OS_PASSWORD=<PASSWORD>
export OS_REGION_NAME=<REGION>

# Keystone v3
export OS_AUTH_URL=<AUTH_URL /v3>
export OS_PASSWORD=<PASSWORD>
export OS_USERNAME=<USERNAME>
export OS_PROJECT_ID=<PROJECT_ID>
export OS_PROJECT_NAME=<PROJECT_NAME>
export OS_REGION_NAME=<REGION_NAME>
export OS_DOMAIN_NAME=<DOMAIN_NAME OR OS_USER_DOMAIN_NAME>

# If you want to test with unsecure certificates
export OS_VERIFY="false"
```

If your Openstack cloud has separated Swift service (SwiftStack or different), you can specify special environment variables for Swift to authenticate it and keep the standard ones for Cinder:

```bash
# Swift with SwiftStack
export OS_SWIFT_AUTH_URL=<AUTH_URL /v2.0>
export OS_SWIFT_PASSWORD=<PASSWORD>
export OS_SWIFT_PROJECT_ID=<PROJECT_ID>
export OS_SWIFT_REGION_NAME=<REGION_NAME>
export OS_SWIFT_TENANT_NAME=<TENANT_NAME>
export OS_SWIFT_USERNAME=<USERNAME>
```

### Install Using Velero CLI

Initialize velero plugin

```bash
# Initialize velero from scratch:
velero install \
       --provider "openstack" \
       --plugins lirt/velero-plugin-for-openstack:v0.2.1 \
       --bucket <BUCKET_NAME> \
       --no-secret

# Or add plugin to existing velero:
velero plugin add lirt/velero-plugin-for-openstack:v0.2.1
```

Change configuration of `backupstoragelocations.velero.io`:

```yaml
 spec:
   objectStorage:
     bucket: <BUCKET_NAME>
   provider: openstack
```

Change configuration of `volumesnapshotlocations.velero.io`:

```yaml
 spec:
   provider: openstack
```

### Install Using Helm Chart

Alternative installation can be done using Helm Charts.

There is an [official helm chart for Velero](https://github.com/vmware-tanzu/helm-charts/) which can be used to install both velero and velero openstack plugin.

To use it, first create `values.yaml` file which will later be used in helm installation (here is just minimal necessary configuration):

```yaml
---
credentials:
  extraSecretRef: "velero-credentials"
configuration:
  provider: openstack
  backupStorageLocation:
    bucket: my-swift-bucket
initContainers:
- name: velero-plugin-openstack
  image: lirt/velero-plugin-for-openstack:v0.2.1
  imagePullPolicy: IfNotPresent
  volumeMounts:
    - mountPath: /target
      name: plugins
snapshotsEnabled: true
backupsEnabled: true
# caCert: <CERT_CONTENTS_IN_BASE64>
```

Make sure that secret `velero-credentials` exists and has proper format and content.

Then install `velero` using command like this:

```bash
helm repo add vmware-tanzu https://vmware-tanzu.github.io/helm-charts
helm repo update
helm upgrade \
     velero \
     vmware-tanzu/velero \
     --install \
     --namespace velero \
     --values values.yaml \
     --version 2.15.0
```

## Volume Backups

Please note two things regarding volume backups:
1. The snapshots are done using flag `--force`. The reason is that volumes in state `in-use` cannot be snapshotted without it (they would need to be detached in advance). In some cases this can make snapshot contents inconsistent.
2. Snapshots in the cinder backend are not always supposed to be used as durable. In some cases for proper availability, the snapshot need to be backed up to off-site storage. Please consult if your cinder backend creates durable snapshots with your cloud provider.

Volume backups with Velero can also be done using [Restic](https://velero.io/docs/main/restic/).

## Build

```bash
# Build code
go mod tidy
go build

# Build image
docker build --file docker/Dockerfile --tag velero-plugin-for-openstack:my-test-tag .
```

## Test

```bash
go test -v ./...
```

## Development

The plugin interface is built based on the [official Velero plugin example](https://github.com/vmware-tanzu/velero-plugin-example).
