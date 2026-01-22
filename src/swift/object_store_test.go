package swift

import (
	"crypto/md5"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/Lirt/velero-plugin-for-openstack/src/testhelper"
	th "github.com/gophercloud/gophercloud/v2/testhelper"
	fakeClient "github.com/gophercloud/gophercloud/v2/testhelper/client"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

const ID = "0123456789"
const tokenResp = `{
    "token": {
        "audit_ids": ["VcxU2JYqT8OzfUVvrjEITQ", "qNUTIJntTzO1-XUk5STybw"],
        "catalog": [
            {
                "endpoints": [
                    {
                        "id": "796186fced4611ee9e2c9cb6d0fbac9d",
                        "interface": "public",
                        "region": "RegionOne",
                        "url": "http://localhost:5000"
                    },
                    {
                        "id": "7c2bb2cced4611ee90c09cb6d0fbac9d",
                        "interface": "internal",
                        "region": "RegionOne",
                        "url": "http://localhost:5000"
                    },
                    {
                        "id": "8080e7b6ed4611ee88be9cb6d0fbac9d",
                        "interface": "admin",
                        "region": "RegionOne",
                        "url": "http://localhost:35357"
                    }
                ],
                "id": "854d03ceed4611ee82b09cb6d0fbac9d",
                "type": "identity",
                "name": "keystone"
            },
            {
                "endpoints": [
                    {
                        "id": "5fb3e04cc47345079bcccfa5a78d4de6",
                        "interface": "internal",
                        "region_id": "myRegion",
                        "url": "http://localhost/v3/955f0136ed4611ee9f489cb6d0fbac9d",
                        "region": "myRegion"
                    },
                    {
                        "id": "d48c520ef7b941c692100f24a1437864",
                        "interface": "public",
                        "region_id": "myRegion",
                        "url": "https://localhost/v3/955f0136ed4611ee9f489cb6d0fbac9d",
                        "region": "myRegion"
                    },
                    {
                        "id": "da15876d31f24af3afc3a69cb918c45f",
                        "interface": "admin",
                        "region_id": "myRegion",
                        "url": "https://localhost/v3/955f0136ed4611ee9f489cb6d0fbac9d",
                        "region": "myRegion"
                    }
                ],
                "id": "439e9f0d9d224b88a9b01774a9948e5e",
                "type": "object-store",
                "name": "swift"
            },
            {
                "endpoints": [
                    {
                        "id": "2bed9ab4ed4111eeb4229cb6d0fbac9d",
                        "interface": "internal",
                        "region_id": "secondRegion",
                        "url": "http://localhost2/v3/4c30519aed4111eeab909cb6d0fbac9d",
                        "region": "secondRegion"
                    },
                    {
                        "id": "3bd7f8caed4111eeb77a9cb6d0fbac9d",
                        "interface": "public",
                        "region_id": "secondRegion",
                        "url": "https://localhost2/v3/4c30519aed4111eeab909cb6d0fbac9d",
                        "region": "secondRegion"
                    },
                    {
                        "id": "46474c98ed4111eeb2839cb6d0fbac9d",
                        "interface": "admin",
                        "region_id": "secondRegion",
                        "url": "https://localhost2/v3/4c30519aed4111eeab909cb6d0fbac9d",
                        "region": "secondRegion"
                    }
                ],
                "id": "4c30519aed4111eeab909cb6d0fbac9d",
                "type": "object-store",
                "name": "swift"
            }
        ],
        "expires_at": "2025-02-27T18:30:59.999999Z",
        "is_domain": false,
        "issued_at": "2025-02-27T16:30:59.999999Z",
        "methods": [
            "password"
        ],
        "project": {
            "domain": {
                "id": "8789d1",
                "name": "domain"
            },
            "id": "04982538-f42b-11ee-a412-9cb6d0fbac9d",
            "name": "project"
        },
        "roles": [
            {
                "id": "86e72a",
                "name": "admin"
            },
            {
                "id": "e4f392",
                "name": "member"
            }
        ],
        "user": {
            "domain": {
                "id": "8789d1",
                "name": "domain"
            },
            "id": "cf78e694-f42a-11ee-bfcc-9cb6d0fbac9d",
            "name": "user",
            "password_expires_at": "2026-11-06T15:32:17.000000"
        }
    }
}`

// TestInit performs standard object store initialization
// which includes creation of auth client, authentication and
// creation of object storage client.
// In this test we use simple clouds.yaml and not override
// any option.
func TestSimpleObjectStorageInit(t *testing.T) {
	// Basic structs
	log := logrus.New()
	config := map[string]string{
		"cloud": "myCloud",
	}
	os := NewObjectStore(log)

	// Create fake provider client for authentication,
	// prepare handler for authentication and redirect
	// provider endpoint to fake client.
	fakeServer := th.SetupHTTP()
	defer fakeServer.Teardown()
	os.provider = fakeClient.ServiceClient(fakeServer).ProviderClient
	os.provider.IdentityEndpoint = fakeServer.Endpoint() + "v3/auth/tokens"

	tempDir, origDir := testhelper.TempCloudsYAML(t, os.provider.IdentityEndpoint)
	defer testhelper.TempCloudsYAMLCleanup(t, tempDir, origDir)

	testhelper.MuxKeystoneVersionDiscovery(fakeServer, fakeServer.Endpoint()+"v3/")
	fakeServer.Mux.HandleFunc("/v3/auth/tokens",
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("X-Subject-Token", ID)

			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, tokenResp)
		},
	)

	// Try to Init block storage. This involves authentication.
	if err := os.Init(config); err != nil {
		t.Error(err)
	}
}

func handleGetObject(t *testing.T, fakeServer th.FakeServer, container, object string, data []byte) {
	fakeServer.Mux.HandleFunc(fmt.Sprintf("/%s/%s", container, object),
		func(w http.ResponseWriter, r *http.Request) {
			th.TestMethod(t, r, http.MethodGet)
			th.TestHeader(t, r, "X-Auth-Token", fakeClient.TokenID)
			th.TestHeader(t, r, "Accept", "application/json")

			hash := md5.New()
			hash.Write(data)
			localChecksum := hash.Sum(nil)

			w.Header().Set("ETag", fmt.Sprintf("%x", localChecksum))
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(data))
		})
}

func handlePutObject(t *testing.T, fakeServer th.FakeServer, container, object string, data []byte) {
	fakeServer.Mux.HandleFunc(fmt.Sprintf("/%s/%s", container, object),
		func(w http.ResponseWriter, r *http.Request) {
			th.TestMethod(t, r, http.MethodPut)
			th.TestHeader(t, r, "X-Auth-Token", fakeClient.TokenID)
			th.TestHeader(t, r, "Accept", "application/json")

			hash := md5.New()
			hash.Write(data)
			localChecksum := hash.Sum(nil)

			w.Header().Set("ETag", fmt.Sprintf("%x", localChecksum))
			w.WriteHeader(http.StatusCreated)
		})
}

func handleObjectExists(t *testing.T, fakeServer th.FakeServer, container, object string) {
	fakeServer.Mux.HandleFunc(fmt.Sprintf("/%s/%s", container, object),
		func(w http.ResponseWriter, r *http.Request) {
			th.TestMethod(t, r, http.MethodHead)
			th.TestHeader(t, r, "X-Auth-Token", fakeClient.TokenID)
			th.TestHeader(t, r, "Accept", "application/json")

			w.WriteHeader(http.StatusOK)
		})
}

func TestPutObject(t *testing.T) {
	fakeServer := th.SetupHTTP()
	defer fakeServer.Teardown()

	container := "testContainer"
	object := "testKey"
	content := "All code is guilty until proven innocent"
	handlePutObject(t, fakeServer, container, object, []byte(content))

	store := ObjectStore{
		client: fakeClient.ServiceClient(fakeServer),
		log:    logrus.New(),
	}
	err := store.PutObject(container, object, strings.NewReader(content))
	assert.Nil(t, err)
}

func TestGetObject(t *testing.T) {
	fakeServer := th.SetupHTTP()
	defer fakeServer.Teardown()

	container := "testContainer"
	object := "testKey"
	content := "All code is guilty until proven innocent"
	handleGetObject(t, fakeServer, container, object, []byte(content))

	store := ObjectStore{
		client: fakeClient.ServiceClient(fakeServer),
		log:    logrus.New(),
	}
	readCloser, err := store.GetObject(container, object)

	if !assert.Nil(t, err) {
		t.FailNow()
	}
	defer readCloser.Close()
}

func TestObjectExists(t *testing.T) {
	fakeServer := th.SetupHTTP()
	defer fakeServer.Teardown()

	container := "testContainer"
	object := "testKey"
	handleObjectExists(t, fakeServer, container, object)

	store := ObjectStore{
		client: fakeClient.ServiceClient(fakeServer),
		log:    logrus.New(),
	}

	_, err := store.ObjectExists(container, object)

	if !assert.Nil(t, err) {
		t.FailNow()
	}
}
