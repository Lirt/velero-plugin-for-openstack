package swift

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Lirt/velero-plugin-swift/src/utils"
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/objectstorage/v1/objects"
	"github.com/sirupsen/logrus"
)

// ObjectStore is swift type that holds client and log
type ObjectStore struct {
	client     *gophercloud.ServiceClient
	provider   *gophercloud.ProviderClient
	log        logrus.FieldLogger
	tempURLKey string
}

// NewObjectStore instantiates a Swift ObjectStore.
func NewObjectStore(log logrus.FieldLogger) *ObjectStore {
	return &ObjectStore{log: log}
}

// Init initializes the plugin. After v0.10.0, this can be called multiple times.
func (o *ObjectStore) Init(config map[string]string) error {
	o.log.Infof("ObjectStore.Init called")

	err := utils.Authenticate(&o.provider, "swift", config, o.log)
	if err != nil {
		return fmt.Errorf("failed to authenticate against OpenStack: %v", err)
	}

	// If we haven't set client before or we use multiple clouds - get new client
	if o.client == nil || config["cloud"] != "" {
		region, ok := os.LookupEnv("OS_SWIFT_REGION_NAME")
		if !ok {
			region, ok = os.LookupEnv("OS_REGION_NAME")
			if !ok {
				if config["region"] != "" {
					region = config["region"]
				} else {
					region = "RegionOne"
				}
			}
		}
		o.client, err = openstack.NewObjectStorageV1(o.provider, gophercloud.EndpointOpts{
			Region: region,
		})
		if err != nil {
			return fmt.Errorf("failed to create swift storage object: %v", err)
		}
		o.log.Infof("Successfully created service client with endpoint %v using region %v", o.client.Endpoint, region)
	}

	// see https://specs.openstack.org/openstack/swift-specs/specs/in_progress/service_token.html
	resellerPrefixes := strings.Split(utils.GetEnv("OS_SWIFT_RESELLER_PREFIXES", "AUTH_"), ",")
	account := utils.GetEnv("OS_SWIFT_ACCOUNT_OVERRIDE", "")
	if account != "" {
		u, err := url.Parse(o.client.Endpoint)
		if err != nil {
			return fmt.Errorf("failed to parse swift storage client endpoint: %v", err)
		}
		u.Path = utils.ReplaceAccount(account, u.Path, resellerPrefixes)
		o.client.Endpoint = u.String()
		o.log.Infof("Successfully overrode service client endpoint with a %v account: %v", account, o.client.Endpoint)
	}

	endpoint := utils.GetEnv("OS_SWIFT_ENDPOINT_OVERRIDE", "")
	if endpoint != "" {
		u, err := url.Parse(endpoint)
		if err != nil {
			return fmt.Errorf("failed to parse swift storage client endpoint: %v", err)
		}
		o.client.Endpoint = u.String()
		o.client.ResourceBase = ""
		o.log.Infof("Successfully overrode service client endpoint: %v", o.client.Endpoint)
	}

	// override the Temp URL key to generate a URL signature
	o.tempURLKey = utils.GetEnv("OS_SWIFT_TEMP_URL_KEY", "")
	if o.tempURLKey != "" {
		o.log.Infof("Successfully overrode Temp URL key")
	}

	return nil
}

// GetObject returns body of Swift object defined by container name and object
func (o *ObjectStore) GetObject(container, object string) (io.ReadCloser, error) {
	log := o.log.WithFields(logrus.Fields{
		"container": container,
		"object":    object,
	})
	log.Infof("GetObject")

	res := objects.Download(o.client, container, object, nil)
	if res.Err != nil {
		return nil, fmt.Errorf("failed to download contents of %q object from %q container: %v", object, container, res.Err)
	}

	return res.Body, nil
}

// PutObject uploads new object into container
func (o *ObjectStore) PutObject(container string, object string, body io.Reader) error {
	log := o.log.WithFields(logrus.Fields{
		"container": container,
		"object":    object,
	})
	log.Infof("PutObject")

	createOpts := objects.CreateOpts{
		Content: body,
	}

	if _, err := objects.Create(o.client, container, object, createOpts).Extract(); err != nil {
		return fmt.Errorf("failed to create new %q object in %q container: %v", object, container, err)
	}

	return nil
}

// ObjectExists does Get operation and validates result or error to find out if object exists
func (o *ObjectStore) ObjectExists(container, object string) (bool, error) {
	log := o.log.WithFields(logrus.Fields{
		"container": container,
		"object":    object,
	})
	log.Infof("ObjectExists")

	res := objects.Get(o.client, container, object, nil)

	if res.Err != nil {
		if _, ok := res.Err.(gophercloud.ErrDefault404); ok {
			log.Infof("%q object doesn't yet exist in %q container.", object, container)
			return false, nil
		}
		return false, fmt.Errorf("cannot Get %q object from %q container: %v", object, container, res.Err)
	}

	return true, nil
}

// ListCommonPrefixes returns list of objects in container, that match specified prefix
func (o *ObjectStore) ListCommonPrefixes(container, prefix, delimiter string) ([]string, error) {
	log := o.log.WithFields(logrus.Fields{
		"container": container,
		"prefix":    prefix,
		"delimiter": delimiter,
	})
	log.Infof("ListCommonPrefixes")

	opts := objects.ListOpts{
		Prefix:    prefix,
		Delimiter: delimiter,
		Full:      true,
	}

	allPages, err := objects.List(o.client, container, opts).AllPages()
	if err != nil {
		return nil, fmt.Errorf("failed to list objects in %q container: %v", container, err)
	}

	allObjects, err := objects.ExtractInfo(allPages)
	if err != nil {
		return nil, fmt.Errorf("failed to extract objects info from %q container: %v", container, err)
	}

	var objNames []string
	for _, object := range allObjects {
		objNames = append(objNames, object.Subdir+object.Name)
	}

	return objNames, nil
}

// ListObjects lists objects with prefix in all containers
func (o *ObjectStore) ListObjects(container, prefix string) ([]string, error) {
	log := o.log.WithFields(logrus.Fields{
		"container": container,
		"prefix":    prefix,
	})
	log.Infof("ListObjects")

	objects, err := o.ListCommonPrefixes(container, prefix, "/")
	if err != nil {
		return nil, fmt.Errorf("failed to list objects in %q container with %q prefix: %v", container, prefix, err)
	}

	return objects, nil
}

// DeleteObject deletes object specified by object from container
func (o *ObjectStore) DeleteObject(container, object string) error {
	log := o.log.WithFields(logrus.Fields{
		"container": container,
		"object":    object,
	})
	log.Infof("DeleteObject")

	_, err := objects.Delete(o.client, container, object, nil).Extract()
	if err != nil {
		return fmt.Errorf("failed to delete %q object from %q container: %v", object, container, err)
	}

	return nil
}

// CreateSignedURL creates temporary URL for object in container
func (o *ObjectStore) CreateSignedURL(container, object string, ttl time.Duration) (string, error) {
	log := o.log.WithFields(logrus.Fields{
		"container": container,
		"object":    object,
	})
	log.Infof("CreateSignedURL")

	url, err := objects.CreateTempURL(o.client, container, object, objects.CreateTempURLOpts{
		Method:     http.MethodGet,
		TTL:        int(ttl.Seconds()),
		TempURLKey: o.tempURLKey,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create temporary URL for %q object in %q container: %v", object, container, err)
	}

	return url, nil
}
