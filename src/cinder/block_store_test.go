package cinder

import (
	"fmt"
	"net/http"
	"testing"

	th "github.com/gophercloud/gophercloud/testhelper"
	fakeClient "github.com/gophercloud/gophercloud/testhelper/client"
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
                        "url": "http://localhost:8776/v3/955f0136ed4611ee9f489cb6d0fbac9d",
                        "region": "myRegion"
                    },
                    {
                        "id": "d48c520ef7b941c692100f24a1437864",
                        "interface": "public",
                        "region_id": "myRegion",
                        "url": "https://localhost:8776/v3/955f0136ed4611ee9f489cb6d0fbac9d",
                        "region": "myRegion"
                    },
                    {
                        "id": "da15876d31f24af3afc3a69cb918c45f",
                        "interface": "admin",
                        "region_id": "myRegion",
                        "url": "https://localhost:8776/v3/955f0136ed4611ee9f489cb6d0fbac9d",
                        "region": "myRegion"
                    }
                ],
                "id": "439e9f0d9d224b88a9b01774a9948e5e",
                "type": "volumev3",
                "name": "cinderv3"
            },
            {
                "endpoints": [
                    {
                        "id": "2bed9ab4ed4111eeb4229cb6d0fbac9d",
                        "interface": "internal",
                        "region_id": "secondRegion",
                        "url": "http://localhost2:8776/v3/4c30519aed4111eeab909cb6d0fbac9d",
                        "region": "secondRegion"
                    },
                    {
                        "id": "3bd7f8caed4111eeb77a9cb6d0fbac9d",
                        "interface": "public",
                        "region_id": "secondRegion",
                        "url": "https://localhost2:8776/v3/4c30519aed4111eeab909cb6d0fbac9d",
                        "region": "secondRegion"
                    },
                    {
                        "id": "46474c98ed4111eeb2839cb6d0fbac9d",
                        "interface": "admin",
                        "region_id": "secondRegion",
                        "url": "https://localhost2:8776/v3/4c30519aed4111eeab909cb6d0fbac9d",
                        "region": "secondRegion"
                    }
                ],
                "id": "4c30519aed4111eeab909cb6d0fbac9d",
                "type": "volumev3",
                "name": "cinderv3"
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
                "name": "example.com"
            },
            "id": "263fa9",
            "name": "project-y"
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
        "service_providers": [
            {
                "auth_url":"https://example.com:5000/v3/OS-FEDERATION/identity_providers/acme/protocols/saml2/auth",
                "id": "sp1",
                "sp_url": "https://example.com:5000/Shibboleth.sso/SAML2/ECP"
            },
            {
                "auth_url":"https://other.example.com:5000/v3/OS-FEDERATION/identity_providers/acme/protocols/saml2/auth",
                "id": "sp2",
                "sp_url": "https://other.example.com:5000/Shibboleth.sso/SAML2/ECP"
            }
        ],
        "user": {
            "domain": {
                "id": "8789d1",
                "name": "example.com"
            },
            "id": "0ca8f6",
            "name": "Jane",
            "password_expires_at": "2026-11-06T15:32:17.000000"
        }
    }
}`

// TestInit performs standard block store initialization
// which includes creation of auth client, authentication and
// creation of block storage client.
// In this test we use simple clouds.yaml and don't override
// any option.
func TestInit(t *testing.T) {
	// Basic structs
	log := logrus.New()
	config := map[string]string{
		"cloud": "myCloud",
	}
	bs := NewBlockStore(log)

	// Create fake provider client for authentication,
	// prepare handler for authentication and redirect
	// provider endpoint to fake client.
	th.SetupPersistentPortHTTP(t, 32498)
	defer th.TeardownHTTP()
	fakeClient.ServiceClient()
	bs.provider = fakeClient.ServiceClient().ProviderClient
	bs.provider.IdentityEndpoint = th.Endpoint()

	th.Mux.HandleFunc("/v3/auth/tokens",
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("X-Subject-Token", ID)

			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, tokenResp)
		},
	)

	// Try to Init block storage. This involves authentication.
	if err := bs.Init(config); err != nil {
		t.Error(err)
	}
}
