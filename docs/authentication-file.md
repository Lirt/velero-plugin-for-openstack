## Authentication using file

You can also authenticate using file in [`clouds.y(a)ml` format](https://docs.openstack.org/python-openstackclient/pike/configuration/index.html#clouds-yaml).

Easiest way is to create file `/etc/openstack/clouds.y(a)ml` with content like this:

```yaml
clouds:
  <CLOUD_NAME_1>:
    region_name: <REGION_NAME>
    auth:
      auth_url: "<AUTH_URL /v3>"
      username: <USERNAME>
      password: <PASSWORD>
      project_name: <PROJECT_NAME>
      project_domain_name: <PROJECT_DOMAIN_NAME>
      user_domain_name: <USER_DOMAIN_NAME>
  <CLOUD_NAME_2>:
    region_name: <REGION_NAME>
    auth:
      auth_url: "<AUTH_URL /v3>"
      username: <USERNAME>
      password: <PASSWORD>
      project_name: <PROJECT_NAME>
      project_domain_name: <PROJECT_DOMAIN_NAME>
      user_domain_name: <USER_DOMAIN_NAME>
```

Or when authenticating using [Application Credentials](https://docs.openstack.org/keystone/queens/user/application_credentials.html#using-application-credentials) use file content like this:

```yaml
clouds:
  <CLOUD_NAME_1>:
    region_name: <REGION_NAME>
    auth:
      auth_url: "<AUTH_URL /v3>"
      application_credential_name: <APPLICATION_CREDENTIAL_NAME>
      application_credential_id: <APPLICATION_CREDENTIAL_ID>
      application_credential_secret: <APPLICATION_CREDENTIAL_SECRET>
  <CLOUD_NAME_2>:
    region_name: <REGION_NAME>
    auth:
      auth_url: "<AUTH_URL /v3>"
      application_credential_name: <APPLICATION_CREDENTIAL_NAME>
      application_credential_id: <APPLICATION_CREDENTIAL_ID>
      application_credential_secret: <APPLICATION_CREDENTIAL_SECRET>
```