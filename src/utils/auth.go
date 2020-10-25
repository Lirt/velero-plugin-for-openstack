package utils

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
)

// Authenticate to Swift
func Authenticate() (*gophercloud.ProviderClient, error) {
	authOpts, err := openstack.AuthOptionsFromEnv()
	if err != nil {
		return nil, err
	}

	pc, err := openstack.NewClient(authOpts.IdentityEndpoint)
	if err != nil {
		return nil, err
	}

	tlsVerify, err := strconv.ParseBool(GetEnv("OS_VERIFY", "true"))
	if err != nil {
		return nil, fmt.Errorf("Cannot parse boolean from OS_VERIFY environment variable: %v", err)
	}

	tlsconfig := &tls.Config{}
	tlsconfig.InsecureSkipVerify = tlsVerify
	transport := &http.Transport{TLSClientConfig: tlsconfig}
	pc.HTTPClient = http.Client{
		Transport: transport,
	}

	if err := openstack.Authenticate(pc, authOpts); err != nil {
		return nil, err
	}

	return pc, nil
}
