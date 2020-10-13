# Swift plugin for Velero

Openstack Swift plugin for velero backups.

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
     bucket: <BUCKET_NAME>
   provider: swift
```

## Test

```bash
go test -v ./...
```

## Build

```bash
# Build code
go mod tidy
go build

# Build image
docker build --file docker/Dockerfile --tag velero-swift:my-test-tag .
```
