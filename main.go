package main

import (
	"github.com/Lirt/velero-plugin-swift/src/cinder"
	"github.com/Lirt/velero-plugin-swift/src/swift"
	"github.com/sirupsen/logrus"
	veleroplugin "github.com/vmware-tanzu/velero/pkg/plugin/framework"
)

func main() {
	veleroplugin.NewServer().
		RegisterObjectStore("openstack", newSwiftObjectStore).
		RegisterVolumeSnapshotter("openstack", newCinderBlockStore).
		Serve()
}

func newSwiftObjectStore(logger logrus.FieldLogger) (interface{}, error) {
	return swift.NewObjectStore(logger), nil
}

func newCinderBlockStore(logger logrus.FieldLogger) (interface{}, error) {
	return cinder.NewBlockStore(logger), nil
}
