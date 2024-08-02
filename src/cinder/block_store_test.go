package cinder

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	th "github.com/gophercloud/gophercloud/testhelper"
	fakeClient "github.com/gophercloud/gophercloud/testhelper/client"
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
const listDetailResponse = `{
  "backups": [
    {
      "id": "289da7f8-6440-407c-9fb4-7db01ec49164",
      "name": "backup-001",
      "volume_id": "521752a6-acf6-4b2d-bc7a-119f9148cd8c",
      "description": "Daily Backup",
      "status": "available",
      "size": 30,
      "created_at": "2017-05-30T03:35:03.000000"
    },
    {
      "id": "96c3bda7-c82a-4f50-be73-ca7621794835",
      "name": "backup-002",
      "volume_id": "76b8950a-8594-4e5b-8dce-0dfa9c696358",
      "description": "Weekly Backup",
      "status": "available",
      "size": 25,
      "created_at": "2017-05-30T03:35:03.000000"
    }
  ],
  "backups_links": [
    {
      "href": "%s/backups/detail?marker=1",
      "rel": "next"
    }
  ]
}
`
const createBackupResponse = `{
    "backup": {
      "volume_id": "1234",
      "name": "backup-001",
      "id": "%s",
      "description": "Daily backup",
      "volume_id": "1234",
      "status": "available",
      "size": 30,
      "created_at": "2017-05-30T03:35:03.000000"
    }
  }
`
const getBackupResponse = `{
  "backup": {
      "volume_id": "1234",
      "name": "backup-001",
      "id": "%s",
      "description": "Daily backup",
      "volume_id": "1234",
      "status": "available",
      "size": 30,
      "created_at": "2017-05-30T03:35:03.000000"
  }
}`
const getVolumeResponse = `{
    "volume": {
    "volume_type": "lvmdriver-1",
    "created_at": "2015-09-17T03:32:29.000000",
    "bootable": "false",
    "name": "vol-001",
    "os-vol-mig-status-attr:name_id": null,
    "consistencygroup_id": null,
    "source_volid": null,
    "os-volume-replication:driver_data": null,
    "multiattach": false,
    "snapshot_id": null,
    "replication_status": "disabled",
    "os-volume-replication:extended_status": null,
    "encrypted": false,
    "availability_zone": "nova",
    "attachments": [{
        "server_id": "83ec2e3b-4321-422b-8706-a84185f52a0a",
        "attachment_id": "05551600-a936-4d4a-ba42-79a037c1-c91a",
        "attached_at": "2016-08-06T14:48:20.000000",
        "host_name": "foobar",
        "volume_id": "%[1]s",
        "device": "/dev/vdc",
        "id": "d6cacb1a-8b59-4c88-ad90-d70ebb82bb75"
    }],
    "id": "%[1]s",
    "size": 75,
    "user_id": "ff1ce52c03ab433aaba9108c2e3ef541",
    "os-vol-tenant-attr:tenant_id": "304dc00909ac4d0da6c62d816bcb3459",
    "os-vol-mig-status-attr:migstat": null,
    "metadata": {},
    "status": "available",
    "volume_image_metadata": {
        "container_format": "bare",
        "image_name": "centos"
    },
    "description": null
    }
}`

func handleListBackupsDetail(t *testing.T) {
	th.Mux.HandleFunc("/backups/detail", func(w http.ResponseWriter, r *http.Request) {
		th.TestMethod(t, r, "GET")
		th.TestHeader(t, r, "X-Auth-Token", fakeClient.TokenID)

		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if err := r.ParseForm(); err != nil {
			t.Errorf("Failed to parse request form %v", err)
		}
		marker := r.Form.Get("marker")
		switch marker {
		case "":
			fmt.Fprintf(w, listDetailResponse, th.Server.URL)
		case "1":
			fmt.Fprintf(w, `{"backups": []}`)
		default:
			t.Fatalf("Unexpected marker: [%s]", marker)
		}
	})
}

func handleGetVolume(t *testing.T, volumeID string) {
	th.Mux.HandleFunc(fmt.Sprintf("/volumes/%s", volumeID), func(w http.ResponseWriter, r *http.Request) {
		th.TestMethod(t, r, "GET")
		th.TestHeader(t, r, "X-Auth-Token", fakeClient.TokenID)

		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, getVolumeResponse, volumeID, volumeID)
	})
}

func handleGetBackup(t *testing.T, backupID string) {
	th.Mux.HandleFunc(fmt.Sprintf("/backups/%s", backupID), func(w http.ResponseWriter, r *http.Request) {
		th.TestMethod(t, r, "GET")
		th.TestHeader(t, r, "X-Auth-Token", fakeClient.TokenID)

		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, getBackupResponse, backupID)
	})
}

// CreateIncrementalBackupRequest represents the top-level structure containing the Backup object.
type CreateIncrementalBackupRequest struct {
	Backup struct {
		Incremental bool `json:"incremental"`
	} `json:"backup"`
}

// TestInit performs standard block store initialization
// which includes creation of auth client, authentication and
// creation of block storage client.
// In this test we use simple clouds.yaml and not override
// any option.
func TestSimpleBlockStorageInit(t *testing.T) {
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

func TestGetVolumeBackups(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	handleListBackupsDetail(t)
	store := BlockStore{
		client: fakeClient.ServiceClient(),
		log:    logrus.New(),
	}

	volumeID := "76b8950a-8594-4e5b-8dce-0dfa9c696358"
	logWithFields := store.log.WithFields(logrus.Fields{"volumeId": volumeID})
	allBackups, err := store.getVolumeBackups(logWithFields, volumeID)

	if !assert.Nil(t, err) {
		t.FailNow()
	}

	numOfBackups := len(allBackups)
	if numOfBackups != 2 {
		t.Errorf("Expected 2 backups, got %d", numOfBackups)
	}
}

func TestCreateBackup(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	backupID := "d32019d3-bc6e-4319-9c1d-6722fc136a22"
	var createRequest *CreateIncrementalBackupRequest

	handleListBackupsDetail(t)
	handleGetBackup(t, backupID)

	th.Mux.HandleFunc("/backups", func(w http.ResponseWriter, r *http.Request) {
		// Reset createRequest for each request.
		createRequest = &CreateIncrementalBackupRequest{}

		th.TestMethod(t, r, "POST")
		th.TestHeader(t, r, "X-Auth-Token", fakeClient.TokenID)
		json.NewDecoder(r.Body).Decode(&createRequest)

		w.WriteHeader(http.StatusAccepted)
		fmt.Fprintf(w, createBackupResponse, backupID)
	})

	store := BlockStore{
		client:            fakeClient.ServiceClient(),
		log:               logrus.New(),
		backupIncremental: true,
		backupTimeout:     3,
	}

	tests := []struct {
		volumeID                 string
		expectedIncrementalValue bool
	}{
		{"521752a6-acf6-4b2d-bc7a-119f9148cd8c", true},  // volume with existing backups
		{"591752a6-acf6-4b2d-bc7a-119f9148cd8c", false}, // volume without existing backups
	}

	for _, tt := range tests {
		handleGetVolume(t, tt.volumeID)
		createdBackupID, err := store.createBackup(tt.volumeID, "default", map[string]string{})

		if createRequest.Backup.Incremental != tt.expectedIncrementalValue {
			t.Errorf("expected incremental backup to be set to %v, got %v", tt.expectedIncrementalValue, createRequest.Backup.Incremental)
		}

		if createdBackupID != backupID {
			t.Errorf("expected created backup ID to be %v, got %v", backupID, createdBackupID)
		}

		if !assert.Nil(t, err) {
			t.FailNow()
		}
	}
}
