package testhelper

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	th "github.com/gophercloud/gophercloud/v2/testhelper"
)

const myCloudYAML = `
clouds:
  myCloud:
    auth:
      user_domain_name: users
      auth_url: %s
      username: user
      password: pass
      project_name: project
      project_domain_name: domain
    # region_name: myRegion
    identity_api_version: 3
`

const keystoneVersionDiscoveryResp = `{
  "versions": {
    "values": [
      {
        "id": "v3.14",
        "status": "stable",
        "updated": "2020-04-07T00:00:00Z",
        "links": [
          {
            "rel": "self",
            "href": "%s"
          }
        ],
        "media-types": [
          {
            "base": "application/json",
            "type": "application/vnd.openstack.identity-v3+json"
          }
        ]
      }
    ]
  }
}`

const manilaVersionDiscoveryResp = `{
  "versions": [
    {
      "id": "v2.0",
      "status": "CURRENT",
      "version": "2.81",
      "min_version": "2.0",
      "updated": "2015-08-27T11:33:21Z",
      "links": [
        {
          "rel": "describedby",
          "type": "text/html",
          "href": "http://docs.openstack.org/"
        },
        {
          "rel": "self",
          "href": "%s"
        }
      ],
      "media-types": [
        {
          "base": "application/json",
          "type": "application/vnd.openstack.share+json;version=1"
        }
      ]
    }
  ]
}`

func TempCloudsYAML(t *testing.T, endpoint string) (string, string) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Error(err)
	}

	tempDir, err := os.MkdirTemp("", "velero-plugin-for-openstack-*")
	if err != nil {
		t.Error(err)
	}

	f, err := os.Create(filepath.Join(tempDir, "clouds.yaml"))
	if err != nil {
		t.Error(err)
	}

	_, err = fmt.Fprintf(f, myCloudYAML, endpoint)
	f.Close()
	if err != nil {
		t.Error(err)
	}

	err = os.Chdir(tempDir)
	if err != nil {
		t.Error(err)
	}

	return tempDir, origDir
}

func TempCloudsYAMLCleanup(t *testing.T, tempDir, origDir string) {
	err := os.Chdir(origDir)
	if err != nil {
		t.Error(err)
	}

	err = os.RemoveAll(tempDir)
	if err != nil {
		t.Error(err)
	}
}

func MuxKeystoneVersionDiscovery(fakeServer th.FakeServer, endpoint string) {
	fakeServer.Mux.HandleFunc("/",
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, keystoneVersionDiscoveryResp, endpoint)
		},
	)
}

func MuxManilaVersionDiscovery(fakeServer th.FakeServer, location, endpoint string) {
	fakeServer.Mux.HandleFunc(location,
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, manilaVersionDiscoveryResp, endpoint)
		},
	)
}
