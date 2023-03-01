package swift

import (
	"crypto/md5"
	"fmt"
	"net/http"
	"strings"
	"testing"

	th "github.com/gophercloud/gophercloud/testhelper"
	fakeClient "github.com/gophercloud/gophercloud/testhelper/client"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func handleGetObject(t *testing.T, container, object string, data []byte) {
	th.Mux.HandleFunc(fmt.Sprintf("/%s/%s", container, object),
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

func handlePutObject(t *testing.T, container, object string, data []byte) {
	th.Mux.HandleFunc(fmt.Sprintf("/%s/%s", container, object),
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

func handleObjectExists(t *testing.T, container, object string) {
	th.Mux.HandleFunc(fmt.Sprintf("/%s/%s", container, object),
		func(w http.ResponseWriter, r *http.Request) {
			th.TestMethod(t, r, http.MethodHead)
			th.TestHeader(t, r, "X-Auth-Token", fakeClient.TokenID)
			th.TestHeader(t, r, "Accept", "application/json")

			w.WriteHeader(http.StatusOK)
		})
}

func TestPutObject(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	container := "testContainer"
	object := "testKey"
	content := "All code is guilty until proven innocent"
	handlePutObject(t, container, object, []byte(content))

	store := ObjectStore{
		client: fakeClient.ServiceClient(),
		log:    logrus.New(),
	}
	err := store.PutObject(container, object, strings.NewReader(content))
	assert.Nil(t, err)
}

func TestGetObject(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	container := "testContainer"
	object := "testKey"
	content := "All code is guilty until proven innocent"
	handleGetObject(t, container, object, []byte(content))

	store := ObjectStore{
		client: fakeClient.ServiceClient(),
		log:    logrus.New(),
	}
	readCloser, err := store.GetObject(container, object)

	if !assert.Nil(t, err) {
		t.FailNow()
	}
	defer readCloser.Close()
}

func TestObjectExists(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	container := "testContainer"
	object := "testKey"
	handleObjectExists(t, container, object)

	store := ObjectStore{
		client: fakeClient.ServiceClient(),
		log:    logrus.New(),
	}

	_, err := store.ObjectExists(container, object)

	if !assert.Nil(t, err) {
		t.FailNow()
	}
}
