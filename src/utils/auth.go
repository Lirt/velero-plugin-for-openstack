package utils

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/utils/client"
	"github.com/gophercloud/utils/openstack/clientconfig"
	"github.com/sirupsen/logrus"
)

// osDebugger satisfies the client.Logger interface to print debug API logs
type osDebugger struct {
	log logrus.FieldLogger
}

func (d osDebugger) Printf(format string, args ...interface{}) {
	d.log.Debugf(format, args...)
}

// Authenticate to OpenStack and write client result to **pc
func Authenticate(pc **gophercloud.ProviderClient, service string, config map[string]string, log logrus.FieldLogger) error {
	var err error
	var clientOpts clientconfig.ClientOpts

	// If we authenticate against multiple clouds, we cannot use reauthentication
	if cloud, ok := config["cloud"]; ok {
		log.Infof("Authentication will be done for cloud %v", cloud)
		clientOpts.Cloud = cloud
	} else {
		// If service client is already initialized and contains auth result
		// we know we were already authenticated
		if *pc != nil {
			clientAuthResult := (*pc).GetAuthResult()
			if clientAuthResult != nil {
				return nil
			}
		}
	}

	if _, ok := os.LookupEnv("OS_SWIFT_AUTH_URL"); ok && service == "swift" {
		log.Infof("Trying to authenticate against SwiftStack using special swift environment variables (see README.md)")

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
		log.Infof("Trying to authenticate against OpenStack using environment variables (including application credentials) or using files ~/.config/openstack/clouds.yaml, /etc/openstack/clouds.yaml and ./clouds.yaml")
		clientOpts.AuthInfo = &clientconfig.AuthInfo{
			AllowReauth: true,
		}
	}

	tlsVerify, err := strconv.ParseBool(GetEnv("TLS_SKIP_VERIFY", "false"))
	if err != nil {
		return fmt.Errorf("cannot parse boolean from TLS_SKIP_VERIFY environment variable: %w", err)
	}

	tlsConfig := &tls.Config{InsecureSkipVerify: tlsVerify}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsConfig

	ao, err := clientconfig.AuthOptions(&clientOpts)
	if err != nil {
		return fmt.Errorf("failed to build auth options: %w", err)
	}

	*pc, err = openstack.NewClient(ao.IdentityEndpoint)
	if err != nil {
		return fmt.Errorf("failed to create a provider: %w", err)
	}
	(*pc).HTTPClient.Transport = transport

	// enable API debug logs
	if log, ok := log.(*logrus.Logger); ok && log.IsLevelEnabled(logrus.DebugLevel) {
		(*pc).HTTPClient.Transport = &client.RoundTripper{
			Rt: transport,
			Logger: osDebugger{log.WithFields(logrus.Fields{
				"source":    "openstack",
				"component": service,
			})},
		}
	}

	// set user agent with a version
	(*pc).UserAgent.Prepend("velero-plugin-for-openstack/" + Version + "@" + GitSHA)

	err = openstack.Authenticate(*pc, *ao)
	if err != nil {
		return fmt.Errorf("failed to authenticate: %w", err)
	}

	log.Infof("Authentication against identity endpoint %v was successful", (*pc).IdentityEndpoint)

	return nil
}
