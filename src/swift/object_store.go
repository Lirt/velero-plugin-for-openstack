package swift

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"time"

	"github.com/Lirt/velero-plugin-swift/src/utils"
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/objectstorage/v1/objects"
	"github.com/sirupsen/logrus"
)

// ObjectStore is swift type that holds client and log
type ObjectStore struct {
	client   *gophercloud.ServiceClient
	provider *gophercloud.ProviderClient
	log      logrus.FieldLogger
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
		return fmt.Errorf("failed to authenticate against Openstack: %v", err)
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

	return nil
}

// GetObject returns body of Swift object defined by bucket name and key
func (o *ObjectStore) GetObject(bucket, key string) (io.ReadCloser, error) {
	log := o.log.WithFields(logrus.Fields{
		"bucket": bucket,
		"key":    key,
	})
	log.Infof("GetObject")

	object := objects.Download(o.client, bucket, key, nil)
	if object.Err != nil {
		return nil, fmt.Errorf("failed to download contents of key %v from bucket %v: %v", key, bucket, object.Err)
	}

	return object.Body, nil
}

// PutObject uploads new object into bucket
func (o *ObjectStore) PutObject(bucket string, key string, body io.Reader) error {
	log := o.log.WithFields(logrus.Fields{
		"bucket": bucket,
		"key":    key,
	})
	log.Infof("PutObject")

	createOpts := objects.CreateOpts{
		Content: body,
	}

	if _, err := objects.Create(o.client, bucket, key, createOpts).Extract(); err != nil {
		return fmt.Errorf("failed to create new object in bucket %v with key %v: %v", bucket, key, err)
	}

	return nil
}

// ObjectExists does Get operation and validates result or error to find out if object exists
func (o *ObjectStore) ObjectExists(bucket, key string) (bool, error) {
	log := o.log.WithFields(logrus.Fields{
		"bucket": bucket,
		"key":    key,
	})
	log.Infof("ObjectExists")

	result := objects.Get(o.client, bucket, key, nil)

	if result.Err != nil {
		errResourceNotFound, err := regexp.MatchString("Resource not found", result.Err.Error())
		if err != nil {
			return false, fmt.Errorf("regexp matching failed: %v", err)
		}
		if errResourceNotFound {
			log.Infof("Key %v in bucket %v doesn't exist yet.", key, bucket)
			return false, nil
		}
		return false, fmt.Errorf("cannot Get key %v in bucket %v: %v", key, bucket, result.Err)
	}

	return true, nil
}

// ListCommonPrefixes returns list of objects in bucket, that match specified prefix
func (o *ObjectStore) ListCommonPrefixes(bucket, prefix, delimiter string) ([]string, error) {
	log := o.log.WithFields(logrus.Fields{
		"bucket":    bucket,
		"delimiter": delimiter,
		"prefix":    prefix,
	})
	log.Infof("ListCommonPrefixes")

	opts := objects.ListOpts{
		Prefix:    prefix,
		Delimiter: delimiter,
		Full:      true,
	}

	allPages, err := objects.List(o.client, bucket, opts).AllPages()
	if err != nil {
		return nil, fmt.Errorf("failed to list objects in bucket %v: %v", bucket, err)
	}

	allObjects, err := objects.ExtractInfo(allPages)
	if err != nil {
		return nil, fmt.Errorf("failed to extract info from objects in bucket %v: %v", bucket, err)
	}

	var objNames []string
	for _, object := range allObjects {
		objNames = append(objNames, object.Subdir+object.Name)
	}

	return objNames, nil
}

// ListObjects lists objects with prefix in all containers
func (o *ObjectStore) ListObjects(bucket, prefix string) ([]string, error) {
	log := o.log.WithFields(logrus.Fields{
		"bucket": bucket,
		"prefix": prefix,
	})
	log.Infof("ListObjects")

	objects, err := o.ListCommonPrefixes(bucket, prefix, "/")
	if err != nil {
		return nil, fmt.Errorf("failed to list objects in bucket %v with prefix %v: %v", bucket, prefix, err)
	}

	return objects, nil
}

// DeleteObject deletes object specified by key from bucket
func (o *ObjectStore) DeleteObject(bucket, key string) error {
	log := o.log.WithFields(logrus.Fields{
		"bucket": bucket,
		"key":    key,
	})
	log.Infof("DeleteObject")

	_, err := objects.Delete(o.client, bucket, key, nil).Extract()
	if err != nil {
		return fmt.Errorf("failed to delete object with key %v from bucket %v: %v", key, bucket, err)
	}

	return nil
}

// CreateSignedURL creates temporary URL for key in bucket
func (o *ObjectStore) CreateSignedURL(bucket, key string, ttl time.Duration) (string, error) {
	log := o.log.WithFields(logrus.Fields{
		"bucket": bucket,
		"key":    key,
	})
	log.Infof("CreateSignedURL")

	url, err := objects.CreateTempURL(o.client, bucket, key, objects.CreateTempURLOpts{
		Method: http.MethodGet,
		TTL:    int(ttl.Seconds()),
	})
	if err != nil {
		return "", fmt.Errorf("failed to create temporary URL for bucket %v with key %v: %v", bucket, key, err)
	}

	return url, nil
}
