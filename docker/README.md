# Supported tags and respective `Dockerfile` links

* [`latest` (Dockerfile)](https://github.com/Lirt/velero-plugin-swift/blob/master/docker/Dockerfile)
* [`v0.1` (Dockerfile)](https://github.com/Lirt/velero-plugin-swift/blob/v0.1/docker/Dockerfile)

# Swift plugin for Velero

Openstack Swift plugin for velero backups.

This image should be used as plugin for [Velero Kubernetes backup solution](https://velero.io/).

Currently it does only backups of Kubernetes resources. Module for volume backups was not implemented yet.

## Configure

Configure velero container with swift authentication environment variables:

```bash
export OS_AUTH_URL=<AUTH_URL /v2.0>
export OS_USERNAME=<USERNAME>
export OS_PASSWORD=<PASSWORD>
export OS_REGION_NAME=<REGION>

# If you want to test with unsecure certificates
export OS_VERIFY="false"
```

Add plugin to velero:

```bash
velero plugin add lirt/velero-plugin-swift:v0.1
```

Change configuration of `backupstoragelocations.velero.io`:

```yaml
 spec:
   objectStorage:
     # Bucket must exist beforehand
     bucket: <BUCKET_NAME>
   provider: swift
```
