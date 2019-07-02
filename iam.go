package main

import (
	"context"
	"github.com/prometheus/common/log"
	"golang.org/x/oauth2/google"
	admin "google.golang.org/api/admin/directory/v1"
	"io/ioutil"
)

// Build and returns an Admin SDK Directory service object authorized with
// the service accounts that act on behalf of the given user.
func getAdminService(serviceAccountKeyfile string, gcpAdminUser string) *admin.Service {
	jsonCredentials, err := ioutil.ReadFile(serviceAccountKeyfile)
	if err != nil {
		promErrors.WithLabelValues("get-serviceaccount-keyfile").Inc()
		log.Errorf("unable to read service account key file %s", err)
		return nil
	}

	config, err := google.JWTConfigFromJSON(jsonCredentials, admin.AdminDirectoryGroupMemberReadonlyScope, admin.AdminDirectoryGroupReadonlyScope)
	if err != nil {
		promErrors.WithLabelValues("get-serviceaccount-secret").Inc()
		log.Errorf("unable to parse service account key file to config: %s", err)
		return nil
	}

	config.Subject = gcpAdminUser
	ctx := context.Background()

	service, err := admin.NewService(ctx)
	if err != nil {
		promErrors.WithLabelValues("get-kube-client").Inc()
		log.Errorf("Unable to retrieve Google Admin Client: %s", err)
		return nil
	}
	return service
}

// Gets recursive the group members by e-mail address
func getMembers(service *admin.Service, groupname string) ([]*admin.Member, error) {
	result, err := service.Members.List(groupname).Do()
	if err != nil {
		promErrors.WithLabelValues("get-members").Inc()
		return nil, err
	}

	var userList []*admin.Member
	for _, member := range result.Members {
		if member.Type == "GROUP" {
			groupMembers, _ := getMembers(service, member.Email)
			userList = append(userList, groupMembers...)
		} else {
			userList = append(userList, member)
		}
	}

	return userList, nil
}

// Remove duplicates from user list
func uniq(list []*admin.Member) []*admin.Member {
	var uniqSet []*admin.Member
loop:
	for _, l := range list {
		for _, x := range uniqSet {
			if l.Email == x.Email {
				continue loop
			}
		}
		uniqSet = append(uniqSet, l)
	}

	return uniqSet
}
