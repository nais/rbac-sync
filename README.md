## rbac-sync
[![License](http://img.shields.io/badge/license-mit-blue.svg?style=flat-square)](https://raw.githubusercontent.com/nais/rbac-sync/master/LICENSE)
[![CircleCI](https://circleci.com/gh/nais/rbac-sync/tree/master.svg?style=svg)](https://circleci.com/gh/nais/rbac-sync/tree/master)
[![Go Report Card](https://goreportcard.com/badge/github.com/nais/rbac-sync)](https://goreportcard.com/report/github.com/nais/rbac-sync)

### What It Does

The application reads information from kubernetes namespace, creates role and rolebinding and adds the google groups members to the newly created role

### Requirements

- The service account's private key file: **-serviceaccount-keyfile** flag
- The email of the a organisational user with access to the Google Admin Directory APIs  **-gcp-admin-user** flag
- The service account must have set domain wide delegation in admin.google.com: https://developers.google.com/admin-sdk/directory/v1/guides/delegation. Manage API access must be configured with the client id, not the service account email address.
- The namespaces to synchronise must have an annotation with the group name and optionally role name and role binding name to generate the role binding. See https://github.com/nais/rbac-sync/examples.

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

