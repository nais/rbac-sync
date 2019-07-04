package main

import (
	"context"
	"fmt"
	"golang.org/x/oauth2/google"
	admin "google.golang.org/api/admin/directory/v1"
	"io/ioutil"
)

type IAMClient interface {
	getMembers(groupEmail string) ([]string, error)
}

type AdminService struct {
	Service *admin.Service
}

type MockAdminService struct{}

func (a MockAdminService) getMembers(groupEmail string) ([]string, error) {
	return []string{"a@b.com", "d@e.fi", "h@i.jp"}, nil
}

func NewAdminService(serviceAccountKeyFile, gcpAdminUser string) (*AdminService, error) {
	service, err := getAdminService(serviceAccountKeyFile, gcpAdminUser)

	if err != nil {
		promErrors.WithLabelValues("new-admin-service").Inc()
		return nil, fmt.Errorf("unable to create admin service: %s", err)
	}

	return &AdminService{Service: service}, nil
}

// Build and returns an Admin SDK Directory service object authorized with
// the service accounts that act on behalf of the given user.
func getAdminService(serviceAccountKeyfile string, gcpAdminUser string) (*admin.Service, error) {
	jsonCredentials, err := ioutil.ReadFile(serviceAccountKeyfile)
	if err != nil {
		return nil, fmt.Errorf("unable to read service account key file %s", err)
	}

	config, err := google.JWTConfigFromJSON(jsonCredentials, admin.AdminDirectoryGroupMemberReadonlyScope, admin.AdminDirectoryGroupReadonlyScope)
	if err != nil {
		return nil, fmt.Errorf("unable to parse service account key file to config: %s", err)
	}

	config.Subject = gcpAdminUser
	ctx := context.Background()

	service, err := admin.NewService(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve Google Admin Client: %s", err)
	}

	return service, nil
}

// Gets group members by e-mail address recursively
func (a AdminService) getMembers(groupEmail string) ([]string, error) {
	members, err := a.getMembersObjects(groupEmail)
	return extractEmail(members), err
}

// Gets group members by e-mail address recursively
func (a AdminService) getMembersObjects(groupEmail string) ([]*admin.Member, error) {
	result, err := a.Service.Members.List(groupEmail).Do()
	if err != nil {
		promErrors.WithLabelValues("get-members").Inc()
		return nil, fmt.Errorf("unable to get members: %s", err)
	}

	var userList []*admin.Member
	for _, member := range result.Members {
		if member.Type == "GROUP" {
			groupMembers, _ := a.getMembersObjects(member.Email)
			userList = append(userList, groupMembers...)
		} else {
			userList = append(userList, member)
		}
	}

	return userList, nil
}

func extractEmail(members []*admin.Member) (emails []string) {
	for _, member := range members {
		emails = append(emails, member.Email)
	}

	return
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
