package manila

import (
	"fmt"
	"net/http"
	"testing"

	th "github.com/gophercloud/gophercloud/v2/testhelper"
	fakeClient "github.com/gophercloud/gophercloud/v2/testhelper/client"
	"github.com/sirupsen/logrus"
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
                        "url": "http://localhost:8786/v3/955f0136ed4611ee9f489cb6d0fbac9d",
                        "region": "myRegion"
                    },
                    {
                        "id": "d48c520ef7b941c692100f24a1437864",
                        "interface": "public",
                        "region_id": "myRegion",
                        "url": "https://localhost:8786/v3/955f0136ed4611ee9f489cb6d0fbac9d",
                        "region": "myRegion"
                    },
                    {
                        "id": "da15876d31f24af3afc3a69cb918c45f",
                        "interface": "admin",
                        "region_id": "myRegion",
                        "url": "https://localhost:8786/v3/955f0136ed4611ee9f489cb6d0fbac9d",
                        "region": "myRegion"
                    }
                ],
                "id": "439e9f0d9d224b88a9b01774a9948e5e",
                "type": "sharev2",
                "name": "manilav2"
            },
            {
                "endpoints": [
                    {
                        "id": "2bed9ab4ed4111eeb4229cb6d0fbac9d",
                        "interface": "internal",
                        "region_id": "secondRegion",
                        "url": "http://localhost2:8786/v3/4c30519aed4111eeab909cb6d0fbac9d",
                        "region": "secondRegion"
                    },
                    {
                        "id": "3bd7f8caed4111eeb77a9cb6d0fbac9d",
                        "interface": "public",
                        "region_id": "secondRegion",
                        "url": "https://localhost2:8786/v3/4c30519aed4111eeab909cb6d0fbac9d",
                        "region": "secondRegion"
                    },
                    {
                        "id": "46474c98ed4111eeb2839cb6d0fbac9d",
                        "interface": "admin",
                        "region_id": "secondRegion",
                        "url": "https://localhost2:8786/v3/4c30519aed4111eeab909cb6d0fbac9d",
                        "region": "secondRegion"
                    }
                ],
                "id": "4c30519aed4111eeab909cb6d0fbac9d",
                "type": "sharev2",
                "name": "manilav2"
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

// TestInit performs standard file share store initialization
// which includes creation of auth client, authentication and
// creation of shared filesystem client.
// In this test we use simple clouds.yaml and not override
// any options.
func TestSimpleSharedFilesystemInit(t *testing.T) {
	// Basic structs
	log := logrus.New()
	config := map[string]string{
		"cloud": "myCloud",
	}
	fs := NewFSStore(log)

	// Create fake provider client for authentication,
	// prepare handler for authentication and redirect
	// provider endpoint to fake client.
	th.SetupPersistentPortHTTP(t, 32499)
	defer th.TeardownHTTP()
	fakeClient.ServiceClient()
	fs.provider = fakeClient.ServiceClient().ProviderClient
	fs.provider.IdentityEndpoint = th.Endpoint()

	th.Mux.HandleFunc("/v3/auth/tokens",
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("X-Subject-Token", ID)

			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, tokenResp)
		},
	)

	// Try to Init block storage. This involves authentication.
	if err := fs.Init(config); err != nil {
		t.Error(err)
	}
}
