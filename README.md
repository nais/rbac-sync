## rbac-sync
[![License](http://img.shields.io/badge/license-mit-blue.svg?style=flat-square)](https://raw.githubusercontent.com/nais/rbac-sync/master/LICENSE)
[![CircleCI](https://circleci.com/gh/nais/rbac-sync/tree/master.svg?style=svg)](https://circleci.com/gh/nais/rbac-sync/tree/master)
[![Go Report Card](https://goreportcard.com/badge/github.com/nais/rbac-sync)](https://goreportcard.com/report/github.com/nais/rbac-sync)

### What It Does

rbac-sync's task is to synchronize the members of a Google IAM group into a Kubernetes rolebinding. 
What group to synchronize, and which role to map is specified as a Namespace annotation. 

#### Namespace configuration

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: myteam
  annotations:
    "rbac-sync.nais.io/group-name": myteam@domain.no # email/name of the google group, that will be synced into rolebinding
    "rbac-sync.nais.io/role-name": team-member # optional, name of role to be mapped into rolebinding
    "rbac-sync.nais.io/rolebinding-name": myteam-members # optional, name of the rolebinding that rbac-sync creates
  ...
```

### Requirements

- The service account's private key file in json format: **-serviceaccount-keyfile** flag
- The email of the a organisational user with access to the Google Admin Directory APIs  **-gcp-admin-user** flag
- The service account must have set domain wide delegation in admin.google.com: https://developers.google.com/admin-sdk/directory/v1/guides/delegation. Manage API access must be configured with the client id, not the service account email address.
- The namespaces to synchronise must have an annotation with the group name and optionally role name and role binding name to generate the role binding. See https://github.com/nais/rbac-sync/examples.
- The role either specified with annotation `rbac-sync.nais.io/role-name` or given as a flag to the rbac-sync binary is assumed to exist in the namespace it will be created. 

### Flags

| Flag                      | Description                                              | Default value |
| :-------------------------| :--------------------------------------------------------| :-------------|
| -serviceaccount-keyfile   | The Path to the Service Account's Private Key file.      |               |
| -gcp-admin-user           | The user to impersonate with access to Google Admin API. |               |
| -bind-address             | The bind address of the application.                     | :8080         |
| -update-interval          | Update interval in seconds.                              | 5m0s          |


### Prometheus metrics

- **rbac_sync_success**: Cumulative number of role update operations.
- **rbac_sync_errors**: Cumulative number of errors during role update operations.
