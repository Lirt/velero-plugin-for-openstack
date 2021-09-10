package utils

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/utils/openstack/clientconfig"
	"github.com/sirupsen/logrus"
)

// Authenticate to Openstack and write client result to **pc
func Authenticate(pc **gophercloud.ProviderClient, service string, log logrus.FieldLogger) error {
	// If service client is already initialized and contains auth result
	// we know we were already authenticated
	if *pc != nil {
		clientAuthResult := (*pc).GetAuthResult()
		if clientAuthResult != nil {
			return nil
		}
	}

	var err error
	var clientOpts clientconfig.ClientOpts

	if _, ok := os.LookupEnv("OS_SWIFT_AUTH_URL"); ok && service == "swift" {
		log.Infof("Authenticating against Swift using special swift environment variables (see README.md)")
		clientOpts.AuthInfo = &clientconfig.AuthInfo{
			ApplicationCredentialID:     os.Getenv("OS_SWIFT_APPLICATION_CREDENTIAL_ID"),
			ApplicationCredentialName:   os.Getenv("OS_SWIFT_APPLICATION_CREDENTIAL_NAME"),
			ApplicationCredentialSecret: os.Getenv("OS_SWIFT_APPLICATION_CREDENTIAL_SECRET"),
			AuthURL:                     os.Getenv("OS_SWIFT_AUTH_URL"),
			Username:                    os.Getenv("OS_SWIFT_USERNAME"),
			UserID:                      os.Getenv("OS_SWIFT_USER_ID"),
			Password:                    os.Getenv("OS_SWIFT_PASSWORD"),
			DomainID:                    os.Getenv("OS_SWIFT_DOMAIN_ID"),
			DomainName:                  os.Getenv("OS_SWIFT_DOMAIN_NAME"),
			ProjectName:                 os.Getenv("OS_SWIFT_PROJECT_NAME"),
			ProjectID:                   os.Getenv("OS_SWIFT_PROJECT_ID"),
			UserDomainName:              os.Getenv("OS_SWIFT_USER_DOMAIN_NAME"),
			UserDomainID:                os.Getenv("OS_SWIFT_USER_DOMAIN_ID"),
			ProjectDomainName:           os.Getenv("OS_SWIFT_PROJECT_DOMAIN_NAME"),
			ProjectDomainID:             os.Getenv("OS_SWIFT_PROJECT_DOMAIN_ID"),
			AllowReauth:                 true,
		}
	} else {
		log.Infof("Trying to authenticate against Openstack using environment variables (including application credentials) or using files ~/.config/openstack/clouds.yaml, /etc/openstack/clouds.yaml and ./clouds.yaml")
		clientOpts.AuthInfo = &clientconfig.AuthInfo{
			AllowReauth: true,
		}
	}

	tlsVerify, err := strconv.ParseBool(GetEnv("TLS_SKIP_VERIFY", "false"))
	if err != nil {
		return fmt.Errorf("cannot parse boolean from TLS_SKIP_VERIFY environment variable: %v", err)
	}

	tlsConfig := &tls.Config{InsecureSkipVerify: tlsVerify}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsConfig
	clientOpts.HTTPClient = &http.Client{Transport: transport}

	*pc, err = clientconfig.AuthenticatedClient(&clientOpts)
	if err != nil {
		return err
	}
	log.Infof("Authentication successful")

	return nil
}
