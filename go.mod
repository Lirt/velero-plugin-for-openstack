module github.com/Lirt/velero-plugin-swift

go 1.14

require (
	github.com/gophercloud/gophercloud v0.12.2
	github.com/sirupsen/logrus v1.6.0
	github.com/stretchr/testify v1.4.0
	github.com/vmware-tanzu/velero v1.4.2
)

replace github.com/gophercloud/gophercloud => github.com/Lirt/gophercloud v0.12.2
