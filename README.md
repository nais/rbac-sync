## rbac-sync
[![License](http://img.shields.io/badge/license-mit-blue.svg?style=flat-square)](https://raw.githubusercontent.com/nais/rbac-sync/master/LICENSE)
[![CircleCI](https://circleci.com/gh/nais/rbac-sync/tree/master.svg?style=svg)](https://circleci.com/gh/nais/rbac-sync/tree/master)
[![Go Report Card](https://goreportcard.com/badge/github.com/nais/rbac-sync)](https://goreportcard.com/report/github.com/nais/rbac-sync)

### What it does

rbac-sync's task is to synchronize the members of a Google IAM group into a Kubernetes rolebinding. 
What group to synchronize, and which role to map is specified as a Namespace annotation. 

#### How it works

On the specified interval, it will:

1. Fetch information about all the namespaces in the cluster
2. Filter those namespaces who has enabled rbac-sync through the `rbac-sync.nais.io/group-name` annotation (see example below)
3. For each of these namespaces, it will fetch the members in the group (configured with `rbac-sync.nais.io/group-name`) from Google Admin and generate a RoleBinding containing these users and map these to the configured role (`rbac-sync.nais.io/role-name` or default value provided as flag)
4. Remove orphan role bindings
5. Create new role bindings
6. Update existing role bindings
7. zZz

#### Example Namespace configuration

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
  - Add manage API client access with correct id and the following API Scopes:
    View group subscriptions on your domain  https://www.googleapis.com/auth/admin.directory.group.member.readonly 
    View groups on your domain  https://www.googleapis.com/auth/admin.directory.group.readonly 
- The namespaces to synchronise must have an annotation with the group name and optionally role name and role binding name to generate the role binding. See https://github.com/nais/rbac-sync/examples.
- The role either specified with annotation `rbac-sync.nais.io/role-name` or given as a flag to the rbac-sync binary is assumed to exist in the namespace it will be created. 

### Flags

```
$ rbac-sync --help 
Usage of rbac-sync
  -bind-address string
        Bind address for application. (default ":8080")
  -debug
        enables debug logging
  -default-role-name string
        Default name for role if not specified in namespace annotation (default "rbacsync-default")
  -default-rolebinding-name string
        Default name for rolebinding if not specified in namespace annotation (default "rbacsync-default")
  -gcp-admin-user string
        The google admin user e-mail address.
  -kubeconfig string
        path to Kubernetes config file
  -mock-iam
        starts rbac-sync with a mocked version of the IAM client
  -serviceaccount-keyfile string
        The path to the service account private key file.
  -update-interval duration
        Update interval in seconds. (default 5m0s)
```

### Development

```
make local # requires a running k8s as current kubeconfig context
```

This will spin up a rbac-sync in debug mode with a mock IAM client 

### Prometheus metrics

- **rbac_sync_errors**: Cumulative number of errors during role update operations.
