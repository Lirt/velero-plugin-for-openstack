# Velero Plugins for Openstack

Openstack Cinder and Swift plugin for [velero](https://github.com/vmware-tanzu/velero/) backups.

## Configure

Configure velero container with your Openstack authentication environment variables:

```bash
export OS_AUTH_URL=<AUTH_URL /v2.0>
export OS_USERNAME=<USERNAME>
export OS_PASSWORD=<PASSWORD>
export OS_REGION_NAME=<REGION>

# If you want to test with unsecure certificates
export OS_VERIFY="false"
```

Initialize velero plugin

```bash
# Initialize velero from scratch:
velero install --provider openstack --plugins lirt/velero-plugin-for-openstack:v0.2.0 --bucket <BUCKET_NAME> --no-secret

# Or add plugin to existing velero:
velero plugin add lirt/velero-plugin-for-openstack:v0.2.0
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
