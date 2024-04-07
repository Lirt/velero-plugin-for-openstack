package swift

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Lirt/velero-plugin-for-openstack/src/utils"
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/objectstorage/v1/objects"
	"github.com/sirupsen/logrus"
)

// ObjectStore is swift type that holds client and log
type ObjectStore struct {
	client        *gophercloud.ServiceClient
	provider      *gophercloud.ProviderClient
	log           logrus.FieldLogger
	tempURLKey    string
	tempURLDigest string
}

// NewObjectStore instantiates a Swift ObjectStore.
func NewObjectStore(log logrus.FieldLogger) *ObjectStore {
	return &ObjectStore{log: log}
}

// Init initializes the plugin. After v0.10.0, this can be called multiple times.
func (o *ObjectStore) Init(config map[string]string) error {
	var region string
	o.log.WithFields(logrus.Fields{
		"config": config,
	}).Info("ObjectStore.Init called")

	err := utils.Authenticate(&o.provider, "swift", config, o.log)
	if err != nil {
		return fmt.Errorf("failed to authenticate against OpenStack in object storage plugin: %w", err)
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
					region = ""
				}
			}
		}
		o.client, err = openstack.NewObjectStorageV1(o.provider, gophercloud.EndpointOpts{
			Region: region,
		})
		if err != nil {
			return fmt.Errorf("failed to create swift storage object: %w", err)
		}
		o.log.WithFields(logrus.Fields{
			"region": region,
		}).Info("Successfully created object storage service client")
	}

	// see https://specs.openstack.org/openstack/swift-specs/specs/in_progress/service_token.html
	resellerPrefixes := strings.Split(utils.GetEnv("OS_SWIFT_RESELLER_PREFIXES", "AUTH_"), ",")
	account := utils.GetEnv("OS_SWIFT_ACCOUNT_OVERRIDE", "")
	if account != "" {
		u, err := url.Parse(o.client.Endpoint)
		if err != nil {
			return fmt.Errorf("failed to parse swift storage client endpoint: %w", err)
		}
		u.Path = utils.ReplaceAccount(account, u.Path, resellerPrefixes)
		o.client.Endpoint = u.String()
		o.log.WithFields(logrus.Fields{
			"region":   region,
			"account":  account,
			"endpoint": o.client.Endpoint,
		}).Info("Successfully overrode object storage service client endpoint by env OS_SWIFT_ACCOUNT_OVERRIDE")
	}

	endpoint := utils.GetEnv("OS_SWIFT_ENDPOINT_OVERRIDE", "")
	if endpoint != "" {
		u, err := url.Parse(endpoint)
		if err != nil {
			return fmt.Errorf("failed to parse swift storage client endpoint: %w", err)
		}
		o.client.Endpoint = u.String()
		o.client.ResourceBase = ""
		o.log.WithFields(logrus.Fields{
			"region":   region,
			"account":  account,
			"endpoint": o.client.Endpoint,
		}).Info("Successfully overrode object storage service client endpoint by env OS_SWIFT_ENDPOINT_OVERRIDE")
	}

	// override the Temp URL hash function
	o.tempURLDigest = utils.GetEnv("OS_SWIFT_TEMP_URL_DIGEST", "")
	if o.tempURLDigest != "" {
		o.log.WithFields(logrus.Fields{
			"region":        region,
			"account":       account,
			"endpoint":      o.client.Endpoint,
			"tempURLDigest": o.tempURLDigest,
		}).Info("Successfully overrode Temp URL digest by env OS_SWIFT_TEMP_URL_DIGEST")
	}

	// override the Temp URL key to generate a URL signature
	o.tempURLKey = utils.GetEnv("OS_SWIFT_TEMP_URL_KEY", "")
	if o.tempURLKey != "" {
		o.log.WithFields(logrus.Fields{
			"region":        region,
			"account":       account,
			"endpoint":      o.client.Endpoint,
			"tempURLDigest": o.tempURLDigest,
		}).Info("Successfully overrode Temp URL key by env OS_SWIFT_TEMP_URL_KEY")
	}

	return nil
}

// GetObject returns body of Swift object defined by container name and object
func (o *ObjectStore) GetObject(container, object string) (io.ReadCloser, error) {
	o.log.WithFields(logrus.Fields{
		"container": container,
		"object":    object,
	}).Info("ObjectStore.GetObject called")

	res := objects.Download(o.client, container, object, nil)
	if res.Err != nil {
		return nil, fmt.Errorf("failed to download contents of %q object from %q container: %w", object, container, res.Err)
	}

	return res.Body, nil
}

// PutObject uploads new object into container
func (o *ObjectStore) PutObject(container string, object string, body io.Reader) error {
	o.log.WithFields(logrus.Fields{
		"container": container,
		"object":    object,
	}).Info("ObjectStore.PutObject called")

	createOpts := objects.CreateOpts{
		Content: body,
	}

	if _, err := objects.Create(o.client, container, object, createOpts).Extract(); err != nil {
		return fmt.Errorf("failed to create new %q object in %q container: %w", object, container, err)
	}

	return nil
}

// ObjectExists does Get operation and validates result or error to find out if object exists
func (o *ObjectStore) ObjectExists(container, object string) (bool, error) {
	logWithFields := o.log.WithFields(logrus.Fields{
		"container": container,
		"object":    object,
	})
	logWithFields.Info("ObjectStore.ObjectExists called")
	res := objects.Get(o.client, container, object, nil)

	if res.Err != nil {
		if _, ok := res.Err.(gophercloud.ErrDefault404); ok {
			logWithFields.Info("Object doesn't yet exist in container")
			return false, nil
		}
		return false, fmt.Errorf("cannot Get %q object from %q container: %w", object, container, res.Err)
	}

	return true, nil
}

// ListCommonPrefixes returns list of objects in container, that match specified prefix
func (o *ObjectStore) ListCommonPrefixes(container, prefix, delimiter string) ([]string, error) {
	o.log.WithFields(logrus.Fields{
		"container": container,
		"prefix":    prefix,
		"delimiter": delimiter,
	}).Info("ObjectStore.ListCommonPrefixes called")

	opts := objects.ListOpts{
		Prefix:    prefix,
		Delimiter: delimiter,
		Full:      true,
	}

	allPages, err := objects.List(o.client, container, opts).AllPages()
	if err != nil {
		return nil, fmt.Errorf("failed to list objects in %q container: %w", container, err)
	}

	allObjects, err := objects.ExtractInfo(allPages)
	if err != nil {
		return nil, fmt.Errorf("failed to extract objects info from %q container: %w", container, err)
	}

	var objNames []string
	for _, object := range allObjects {
		objNames = append(objNames, object.Subdir+object.Name)
	}

	return objNames, nil
}

// ListObjects lists objects with prefix in all containers
func (o *ObjectStore) ListObjects(container, prefix string) ([]string, error) {
	o.log.WithFields(logrus.Fields{
		"container": container,
		"prefix":    prefix,
	}).Info("ObjectStore.ListObjects called")

	objects, err := o.ListCommonPrefixes(container, prefix, "/")
	if err != nil {
		return nil, fmt.Errorf("failed to list objects in %q container with %q prefix: %w", container, prefix, err)
	}

	return objects, nil
}

// DeleteObject deletes object specified by object from container
func (o *ObjectStore) DeleteObject(container, object string) error {
	logWithFields := o.log.WithFields(logrus.Fields{
		"container": container,
		"object":    object,
	})
	logWithFields.Info("ObjectStore.DeleteObject called")

	_, err := objects.Delete(o.client, container, object, nil).Extract()
	if err != nil {
		if _, ok := err.(gophercloud.ErrDefault404); ok {
			logWithFields.Info("object is already deleted")
			return nil
		}
		return fmt.Errorf("failed to delete %q object from %q container: %w", object, container, err)
	}

	return nil
}

// CreateSignedURL creates temporary URL for object in container
func (o *ObjectStore) CreateSignedURL(container, object string, ttl time.Duration) (string, error) {
	o.log.WithFields(logrus.Fields{
		"container": container,
		"object":    object,
		"ttl":       ttl,
	}).Info("ObjectStore.CreateSignedURL called")

	url, err := objects.CreateTempURL(o.client, container, object, objects.CreateTempURLOpts{
		Method:     http.MethodGet,
		TTL:        int(ttl.Seconds()),
		TempURLKey: o.tempURLKey,
		Digest:     o.tempURLDigest,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create temporary URL for %q object in %q container: %w", object, container, err)
	}

	return url, nil
}
