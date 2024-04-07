## Installation Using Helm Chart

Full Velero and Openstack plugin installation can be done using Helm Chart.

There is an [official helm chart for Velero](https://github.com/vmware-tanzu/helm-charts/) which can be used to install both velero and velero openstack plugin.

To use it, first create `values.yaml` file which will later be used in helm installation (here is just minimal necessary configuration):

```yaml
---
credentials:
  extraSecretRef: "velero-credentials"
configuration:
  backupStorageLocation:
  - name: swift
    provider: community.openstack.org/openstack
    bucket: my-swift-container
    # caCert: <CERT_CONTENTS_IN_BASE64>
    # # Optional config
    # config:
    #   cloud: cloud1
    #   region: fra
    #   # If you want to enable restic you need to set resticRepoPrefix to this value:
    #   #   resticRepoPrefix: swift:<CONTAINER_NAME>:/<PATH>
    #   resticRepoPrefix: swift:my-awesome-container:/restic # Example
  volumeSnapshotLocation:
  # for Cinder block storage
  - name: cinder
    provider: community.openstack.org/openstack-cinder
    config:
      # optional Cloud:
      #   in case clouds.yaml is used as authentication method, cloud allows
      #   user to select which cloud from the clouds.yaml to use for volume backups
      cloud: ""
      # optional Region:
      #   in case multiple regions exist in a single cloud, select which region
      #   will be used for cinder volume backups.
      region: ""
      # optional snapshot method:
      # * "snapshot" is a default cinder snapshot method
      # * "clone" is for a full volume clone instead of a snapshot allowing the
      # source volume to be deleted
      # * "backup" is for a full volume backup uploaded to a Cinder backup
      # allowing the source volume to be deleted (EXPERIMENTAL)
      # * "image" is for a full volume backup uploaded to a Glance image
      # allowing the source volume to be deleted (EXPERIMENTAL)
      # requires the "enable_force_upload" Cinder option to be enabled on the server
      method: snapshot
      # optional resource readiness timeouts in Golang time format: https://pkg.go.dev/time#ParseDuration
      # (default: 5m)
      volumeTimeout: 5m
      snapshotTimeout: 5m
      cloneTimeout: 5m
      backupTimeout: 5m
      imageTimeout: 5m
      # ensures that the Cinder volume/snapshot is removed
      # if an original snapshot volume was marked to be deleted, the volume may
      # end up in "error_deleting" status.
      # if the volume/snapshot is in "error_deleting" status, the plugin will try to reset
      # its status (usually extra admin permissions are required) and delete it again
      # within the defined "snapshotTimeout" or "cloneTimeout"
      ensureDeleted: "true"
      # a delay to wait between delete/reset actions when "ensureDeleted" is enabled
      ensureDeletedDelay: 10s
      # deletes all dependent volume resources (i.e. snapshots) before deleting
      # the clone volume (works only, when a snapshot method is set to clone)
      cascadeDelete: "true"
  # for Manila shared filesystem storage
  - name: manila
    provider: community.openstack.org/openstack-manila
    config:
      # optional Cloud:
      #   in case clouds.yaml is used as authentication method, cloud allows user
      #   to select which cloud from the clouds.yaml to use for manila share backups
      cloud: ""
      # optional Region:
      #   in case multiple regions exist in a single cloud, select which region
      #   will be used for manila share backups.
      region: ""
      # optional snapshot method:
      # * "snapshot" is a default manila snapshot method
      # * "clone" is for a full share clone instead of a snapshot allowing the
      # source share to be deleted
      method: snapshot
      # optional Manila CSI driver name (default: nfs.manila.csi.openstack.org)
      driver: ceph.manila.csi.openstack.org
      # optional resource readiness timeouts in Golang time format: https://pkg.go.dev/time#ParseDuration
      # (default: 5m)
      shareTimeout: 5m
      snapshotTimeout: 5m
      cloneTimeout: 5m
      replicaTimeout: 5m
      # ensures that the Manila share/snapshot/replica is removed
      # this is a workaround to the https://bugs.launchpad.net/manila/+bug/2025641 and
      # https://bugs.launchpad.net/manila/+bug/1960239 bugs
      # if the share/snapshot/replica is in "error_deleting" status, the plugin will try
      # to reset its status (usually extra admin permissions are required) and delete it
      # again within the defined "cloneTimeout", "snapshotTimeout" or "replicaTimeout"
      ensureDeleted: "true"
      # a delay to wait between delete/reset actions when "ensureDeleted" is enabled
      ensureDeletedDelay: 10s
      # deletes all dependent share resources (i.e. snapshots, replicas) before deleting
      # the clone share (works only, when a snapshot method is set to clone)
      cascadeDelete: "true"
      # enforces availability zone checks when the availability zone of a
      # snapshot/share differs from the Velero metadata
      enforceAZ: "true"
initContainers:
- name: velero-plugin-openstack
  image: lirt/velero-plugin-for-openstack:v0.6.0
  imagePullPolicy: IfNotPresent
  volumeMounts:
    - mountPath: /target
      name: plugins
snapshotsEnabled: true
backupsEnabled: true
# Optionally enable restic
# deployRestic: true
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
     --version 4.0.1
```
