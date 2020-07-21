package main

import (
	swift "github.com/Lirt/velero-plugin-swift/src"
	"github.com/sirupsen/logrus"
	veleroplugin "github.com/vmware-tanzu/velero/pkg/plugin/framework"
)

func main() {
	veleroplugin.NewServer().
		RegisterObjectStore("velero.io/swift", newSwiftObjectStore).
		Serve()
}

func newSwiftObjectStore(logger logrus.FieldLogger) (interface{}, error) {
	return swift.NewObjectStore(logger), nil
}
