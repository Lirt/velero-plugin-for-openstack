package main

import (
	"github.com/Lirt/velero-plugin-for-openstack/src/cinder"
	"github.com/Lirt/velero-plugin-for-openstack/src/swift"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	veleroplugin "github.com/vmware-tanzu/velero/pkg/plugin/framework"
)

func main() {
	veleroplugin.NewServer().
		BindFlags(pflag.CommandLine).
		RegisterObjectStore("community.openstack.org/openstack", newSwiftObjectStore).
		RegisterVolumeSnapshotter("community.openstack.org/openstack", newCinderBlockStore).
		Serve()
}

func newSwiftObjectStore(logger logrus.FieldLogger) (interface{}, error) {
	return swift.NewObjectStore(logger), nil
}

func newCinderBlockStore(logger logrus.FieldLogger) (interface{}, error) {
	return cinder.NewBlockStore(logger), nil
}
