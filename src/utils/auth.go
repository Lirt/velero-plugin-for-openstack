package utils

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/sirupsen/logrus"
)

// Authenticate to Openstack and write client result to **pc
func Authenticate(pc **gophercloud.ProviderClient, log logrus.FieldLogger) error {
	// If service client is already initialized and contains auth result
	// we know we were already authenticated, or the client was reauthenticated
	// using AllowReauth
	if *pc != nil {
		clientAuthResult := (*pc).GetAuthResult()
		if clientAuthResult != nil {
			return nil
		}
	}

	var err error
	var authOpts gophercloud.AuthOptions

	_, ok := os.LookupEnv("OS_SWIFT_AUTH_URL")
	if ok {
		log.Infof("Authenticating against Swift using environment variables")
		authOpts = gophercloud.AuthOptions{
			IdentityEndpoint: os.Getenv("OS_SWIFT_AUTH_URL"),
			Username:         os.Getenv("OS_SWIFT_USERNAME"),
			UserID:           os.Getenv("OS_SWIFT_USER_ID"),
			Password:         os.Getenv("OS_SWIFT_PASSWORD"),
			Passcode:         os.Getenv("OS_SWIFT_PASSCODE"),
			DomainID:         os.Getenv("OS_SWIFT_DOMAIN_ID"),
			DomainName:       os.Getenv("OS_SWIFT_DOMAIN_NAME"),
			TenantID:         os.Getenv("OS_SWIFT_TENANT_ID"),
			TenantName:       os.Getenv("OS_SWIFT_TENANT_NAME"),
		}
	} else {
		log.Infof("Authenticating against Openstack using environment variables")
		authOpts, err = openstack.AuthOptionsFromEnv()
		if err != nil {
			return err
		}
	}

	authOpts.AllowReauth = true

	*pc, err = openstack.NewClient(authOpts.IdentityEndpoint)
	if err != nil {
		return err
	}

	tlsVerify, err := strconv.ParseBool(GetEnv("OS_VERIFY", "true"))
	if err != nil {
		return fmt.Errorf("cannot parse boolean from OS_VERIFY environment variable: %v", err)
	}

	tlsconfig := &tls.Config{}
	tlsconfig.InsecureSkipVerify = tlsVerify
	transport := &http.Transport{TLSClientConfig: tlsconfig}
	(*pc).HTTPClient = http.Client{
		Transport: transport,
	}

	if err := openstack.Authenticate(*pc, authOpts); err != nil {
		return err
	}
	log.Infof("Authentication successful")

	return nil
}
