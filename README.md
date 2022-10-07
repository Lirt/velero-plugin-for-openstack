# Velero Plugin for Openstack

Openstack Cinder and Swift plugin for [velero](https://github.com/vmware-tanzu/velero/) backups.

This plugin is [included as community supported plugin by Velero organization](https://velero.io/plugins/).

## Table of Contents

- [Velero Plugin for Openstack](#velero-plugin-for-openstack)
  - [Compatibility](#compatibility)
  - [Openstack Authentication Configuration](#openstack-authentication-configuration)
    - [Authentication using environment variables](#authentication-using-environment-variables)
    - [Authentication using file](#authentication-using-file)
  - [Installation](#installation)
    - [Install using Velero CLI](#install-using-velero-cli)
    - [Install Using Helm Chart](#install-using-helm-chart)
  - [Volume Backups](#volume-backups)
  - [Known Issues](#known-issues)
  - [Build](#build)
  - [Test](#test)
  - [Development](#development)

## Compatibility

Below is a matrix of plugin versions and Velero versions for which the compatibility is tested and guaranteed.

| Plugin Version | Velero Version |
| :------------- | :------------- |
| v0.3.x         | v1.4.x, v1.5.x, v1.6.x, v1.7.x, v1.8.x |
| v0.2.x         | v1.4.x, v1.5.x |
| v0.1.x         | v1.4.x, v1.5.x |

## Openstack Authentication Configuration

The order of authentication methods is following:
1. Authentication using environment variables takes precedence (including [Application Credentials](https://docs.openstack.org/keystone/queens/user/application_credentials.html#using-application-credentials)). You **must not** set env. variable `OS_CLOUD` when you want to authenticate using env. variables because authenticator will try to look for `clouds.y(a)ml` file and use it.
1. Authentication using files is second option. Note: you will also need to set `OS_CLOUD` environment variable to tell which cloud from `clouds.y(a)ml` will be used:
  1. If `OS_CLIENT_CONFIG_FILE` env. variable is specified, code will authenticate using this file.
  1. Look for file `clouds.y(a)ml` in current directory.
  1. Look for file in `~/.config/openstack/clouds.y(a)ml`.
  1. Look for file in `/etc/openstack/clouds.y(a)ml`.

For authentication using application credentials you need to first create them using openstack CLI command such as `openstack application credential create <NAME>`.

### Authentication using Environment Variables

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

# Keystone v3 with Authentication Credentials
export OS_AUTH_URL=<AUTH_URL /v3>
export OS_APPLICATION_CREDENTIAL_ID=<APP_CRED_ID>
export OS_APPLICATION_CREDENTIAL_NAME=<APP_CRED_NAME>
export OS_APPLICATION_CREDENTIAL_SECRET=<APP_CRED_SECRET>

# If you want to test with unsecure certificates
export OS_VERIFY="false"
export TLS_SKIP_VERIFY="true"
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

### Authentication using file

You can also authenticate using file in [`clouds.y(a)ml` format](https://docs.openstack.org/python-openstackclient/pike/configuration/index.html#clouds-yaml).

Easiest way is to create file `/etc/openstack/clouds.y(a)ml` with content like this:

```yaml
clouds:
  <CLOUD_NAME>:
    region_name: <REGION_NAME>
    auth:
      auth_url: "<AUTH_URL /v3>"
      username: <USERNAME>
      password: <PASSWORD>
      project_name: <PROJECT_NAME>
      project_domain_name: <PROJECT_DOMAIN_NAME>
      user_domain_name: <USER_DOMAIN_NAME>
```

Or when authenticating using [Application Credentials](https://docs.openstack.org/keystone/queens/user/application_credentials.html#using-application-credentials) use file content like this:

```yaml
clouds:
  <CLOUD_NAME>:
    region_name: <REGION_NAME>
    auth:
      auth_url: "<AUTH_URL /v3>"
      application_credential_name: <APPLICATION_CREDENTIAL_NAME>
      application_credential_id: <APPLICATION_CREDENTIAL_ID>
      application_credential_secret: <APPLICATION_CREDENTIAL_SECRET>
```

## Installation

### Install using Velero CLI

Initialize velero plugin:

```bash
# Initialize velero from scratch:
velero install \
       --provider "community.openstack.org/openstack" \
       --plugins lirt/velero-plugin-for-openstack:v0.3.1 \
       --bucket <BUCKET_NAME> \
       --no-secret

# Or add plugin to existing velero:
velero plugin add lirt/velero-plugin-for-openstack:v0.3.1
```

Note: If you want to use plugin built for `arm` or `arm64` architecture, you can use tag such as this `lirt/velero-plugin-for-openstack:v0.3.1-arm64`.

Change configuration of `backupstoragelocations.velero.io`:

```yaml
 spec:
   objectStorage:
     bucket: <BUCKET_NAME>
   provider: community.openstack.org/openstack
```

Change configuration of `volumesnapshotlocations.velero.io`:

```yaml
 spec:
   provider: community.openstack.org/openstack
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
  provider: community.openstack.org/openstack
  backupStorageLocation:
    bucket: my-swift-bucket
    # caCert: <CERT_CONTENTS_IN_BASE64>
initContainers:
- name: velero-plugin-openstack
  image: lirt/velero-plugin-for-openstack:v0.3.1
  imagePullPolicy: IfNotPresent
  volumeMounts:
    - mountPath: /target
      name: plugins
snapshotsEnabled: true
backupsEnabled: true
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
     --version 2.29.8
```

## Volume Backups

Please note two things regarding volume backups:
1. The snapshots are done using flag `--force`. The reason is that volumes in state `in-use` cannot be snapshotted without it (they would need to be detached in advance). In some cases this can make snapshot contents inconsistent.
2. Snapshots in the cinder backend are not always supposed to be used as durable. In some cases for proper availability, the snapshot need to be backed up to off-site storage. Please consult if your cinder backend creates durable snapshots with your cloud provider.

Volume backups with Velero can also be done using [Restic](https://velero.io/docs/main/restic/).

Alternative Kubernetes native solution (GA since 1.20) for volume snapshots (not backups) are [VolumeSnapshots](https://kubernetes.io/docs/concepts/storage/volume-snapshots/) using [snapshot-controller](https://kubernetes-csi.github.io/docs/snapshot-controller.html).

## Known Issues

- [Incompatibility with Cinder version 13.0.0 (Rocky)](https://github.com/Lirt/velero-plugin-for-openstack/issues/20)

## Build

```bash
# Build code
go mod tidy
go build

# Build image
docker buildx --file docker/Dockerfile \
              --platform "linux/amd64" \
              --tag lirt/velero-plugin-for-openstack:v0.3.1 \
              --load \
              .

# Build and push image for linux amd64, arm64, arm
docker buildx build \
              --file docker/Dockerfile \
              --platform linux/amd64,linux/arm/v6,linux/arm/v7,linux/arm64 \
              --tag lirt/velero-plugin-for-openstack:v0.3.1 \
              --push \
              .

# Build one platform by one for local build.
# Building all with --load cannot be done until docker fixes it.
# GitHub issue: https://github.com/docker/buildx/issues/59
for platform in linux/amd64 linux/arm/v6 linux/arm/v7 linux/arm64; do
    docker buildx build \
                  --file docker/Dockerfile \
                  --tag lirt/velero-plugin-for-openstack:v0.3.1 \
                  --platform "${platform}" \
                  --load \
                  .
done
```

## Test

```bash
go test -v ./...
```

## Development

The plugin interface is built based on the [official Velero plugin example](https://github.com/vmware-tanzu/velero-plugin-example).
