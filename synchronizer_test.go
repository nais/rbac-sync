package main

import (
	"github.com/stretchr/testify/assert"
	"testing"
	"time"

	rbacv1beta1 "k8s.io/api/rbac/v1beta1"
)

var (
	rbacConfig = NewSynchronizer(nil, time.Duration(time.Hour), "", "", "", "").NewRbacConfiguration(namespace)
)

func TestNewRbacConfiguration(t *testing.T) {
	assert.NotNil(t, rbacConfig)
	assert.Equal(t, namespace.Name, rbacConfig.namespace)
	assert.Equal(t, "nais:teammember", rbacConfig.rolebindingName)
	assert.Equal(t, "nais:developer", rbacConfig.roleName)
	assert.Equal(t, groupName, rbacConfig.groupName)
}

func TestGetRoleBindingWithSubjects(t *testing.T) {
	var subjects []rbacv1beta1.Subject
	subs := append(subjects, getSubjectWithEmail("testuser@test.com"))
	rolebinding := getRoleBindingWithSubjects(rbacConfig, subs)

	assert.NotNil(t, rolebinding)
	assert.Equal(t, namespace.Name, rolebinding.Namespace)
	assert.Equal(t, "nais:teammember", rolebinding.Name)
	assert.Equal(t, "nais:developer", rolebinding.RoleRef.Name)
}

func TestGetSubjectFromEmail(t *testing.T) {
	email := "testuser@test.com"
	subject := getSubjectWithEmail(email)

	assert.NotNil(t, subject)
	assert.Equal(t, "User", subject.Kind)
	assert.Equal(t, "rbac.authorization.k8s.io", subject.APIGroup)
	assert.Equal(t, email, subject.Name)

}
