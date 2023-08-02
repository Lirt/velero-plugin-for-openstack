## Authentication using Environment Variables

Configure velero container with your OpenStack authentication environment variables:

```bash
# Keystone v2.0
export OS_AUTH_URL=<AUTH_URL /v2.0>
export OS_USERNAME=<USERNAME>
export OS_PASSWORD=<PASSWORD>
export OS_REGION_NAME=<REGION>

# Keystone v3
export OS_AUTH_URL=<AUTH_URL /v3>
export OS_PASSWORD=<PASSWORD>
export OS_USERNAME=<USERNAME>
export OS_PROJECT_ID=<PROJECT_ID>
export OS_PROJECT_NAME=<PROJECT_NAME>
export OS_REGION_NAME=<REGION_NAME>
export OS_DOMAIN_NAME=<DOMAIN_NAME OR OS_USER_DOMAIN_NAME>

# Keystone v3 with Authentication Credentials
export OS_AUTH_URL=<AUTH_URL /v3>
export OS_APPLICATION_CREDENTIAL_ID=<APP_CRED_ID>
export OS_APPLICATION_CREDENTIAL_NAME=<APP_CRED_NAME>
export OS_APPLICATION_CREDENTIAL_SECRET=<APP_CRED_SECRET>

# If you want to test with unsecure certificates
export OS_VERIFY="false"
export TLS_SKIP_VERIFY="true"

# A custom hash function to use for Temp URL generation
export OS_SWIFT_TEMP_URL_DIGEST=sha256
# If you want to override Swift account ID
export OS_SWIFT_ACCOUNT_OVERRIDE=<NEW_PROJECT_ID>
# In case if you have non-standard reseller prefixes
export OS_SWIFT_RESELLER_PREFIXES=AUTH_,SERVICE_
# A valid Temp URL key must be specified, when overriding the Swift account ID
export OS_SWIFT_TEMP_URL_KEY=secret-key

# If you want to completely override Swift endpoint URL
# Has a higher priority over the OS_SWIFT_ACCOUNT_OVERRIDE
export OS_SWIFT_ENDPOINT_OVERRIDE=http://my-local/v1/swift
```

If your OpenStack cloud has separated Swift service (SwiftStack or different), you can specify special environment variables for Swift to authenticate it and keep the standard ones for Cinder and Manila:

```bash
# Swift with SwiftStack
export OS_SWIFT_AUTH_URL=<AUTH_URL /v2.0>
export OS_SWIFT_PASSWORD=<PASSWORD>
export OS_SWIFT_PROJECT_ID=<PROJECT_ID>
export OS_SWIFT_REGION_NAME=<REGION_NAME>
export OS_SWIFT_TENANT_NAME=<TENANT_NAME>
export OS_SWIFT_USERNAME=<USERNAME>
```

This option does not support using multiple clouds (or BSLs) for backups.