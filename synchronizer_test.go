package main

import (
	"time"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/rbac/v1"
	"k8s.io/client-go/kubernetes/fake"

	"testing"
)

func TestRolebindings(t *testing.T) {
	t.Run("finds orphan role bindings", func(t *testing.T) {
		r1 := roleBinding("a", "ns1", "admin", nil)
		r2 := roleBinding("b", "ns2", "admin", nil)

		roleBindings := diff([]v1.RoleBinding{r1}, []v1.RoleBinding{r1, r2})
		assert.Equal(t, len(roleBindings), 1)
		assert.Equal(t, roleBindings[0], r2)
	})

	t.Run("finds updated role bindings when role has changed", func(t *testing.T) {
		aWithAdmin := roleBinding("a", "ns1", "admin", nil)
		aWithEdit := roleBinding("a", "ns1", "edit", nil)

		toUpdate := roleBindingsToUpdate([]v1.RoleBinding{aWithAdmin}, []v1.RoleBinding{aWithEdit})

		assert.Equal(t, len(toUpdate), 1)
		assert.Equal(t, toUpdate[0], aWithAdmin)
	})

	t.Run("finds no role bindings when subjects are just out of order", func(t *testing.T) {
		r1 := roleBinding("a", "ns1", "admin", []string{"x", "y", "z"})
		r2 := roleBinding("a", "ns1", "admin", []string{"z", "x", "y"})

		toUpdate := roleBindingsToUpdate([]v1.RoleBinding{r1}, []v1.RoleBinding{r2})
		assert.Equal(t, len(toUpdate), 0)
	})

	t.Run("finds updated role bindings when subject has been added", func(t *testing.T) {
		r1 := roleBinding("a", "ns1", "admin", []string{"x", "y", "z"})
		r2 := roleBinding("a", "ns1", "admin", []string{"x", "y"})

		toUpdate := roleBindingsToUpdate([]v1.RoleBinding{r1}, []v1.RoleBinding{r2})
		assert.Equal(t, len(toUpdate), 1)
		assert.Equal(t, toUpdate[0], r1)
	})

	t.Run("finds updated role bindings when subject has been removed", func(t *testing.T) {
		r1 := roleBinding("a", "ns1", "admin", []string{"x", "y"})
		r2 := roleBinding("a", "ns1", "admin", []string{"x", "y", "z"})

		toUpdate := roleBindingsToUpdate([]v1.RoleBinding{r1}, []v1.RoleBinding{r2})
		assert.Equal(t, len(toUpdate), 1)
		assert.Equal(t, toUpdate[0], r1)
	})

	t.Run("finds updated role bindings when subject has changed", func(t *testing.T) {
		r1 := roleBinding("a", "ns1", "admin", []string{"x", "y", "z"})
		r2 := roleBinding("a", "ns1", "admin", []string{"a", "x", "y"})

		toUpdate := roleBindingsToUpdate([]v1.RoleBinding{r1}, []v1.RoleBinding{r2})
		assert.Equal(t, len(toUpdate), 1)
		assert.Equal(t, toUpdate[0], r1)
	})

	t.Run("errors when not finding any matching role bindings", func(t *testing.T) {
		roleBindings := []v1.RoleBinding{roleBinding("a", "ns2", "", nil)}
		_, err := getMatchingRoleBinding(roleBinding("a", "ns1", "", nil), roleBindings)
		assert.Error(t, err)
	})

	t.Run("creates new role bindings", func(t *testing.T) {
		s := getMockSynchronizer()
		rolebindings := []v1.RoleBinding{roleBinding("a", "ns1", "admin", []string{"x", "y", "z"}),
			roleBinding("b", "ns2", "admin", []string{"x", "y", "z"})}

		err := s.createRoleBindings(rolebindings)
		assert.NoError(t, err)
	})

	t.Run("error when creating identical role bindings", func(t *testing.T) {
		s := getMockSynchronizer()
		rolebindingsWithError := []v1.RoleBinding{roleBinding("a", "ns1", "admin", []string{"x", "y", "z"}),
			roleBinding("a", "ns1", "admin", []string{"x", "y", "z"})}

		error := s.createRoleBindings(rolebindingsWithError)
		assert.Error(t, error)
	})

	t.Run("test subject diff evaluator", func(t *testing.T) {
		s1 := []v1.Subject{v1.Subject{"User", RBACAPIGroup, "testuser@test.domain", "ns1"}}
		s2 := []v1.Subject{v1.Subject{"User", RBACAPIGroup, "testuser@test.domain", "ns1"},
			v1.Subject{"User", RBACAPIGroup, "testuser2@test.domain", "ns2"}}
		s3 := []v1.Subject{v1.Subject{"User", RBACAPIGroup, "testuser@test.domain", "ns1"},
			v1.Subject{"User", RBACAPIGroup, "testuser2@test.domain", "ns2"}}
		s4 := []v1.Subject{v1.Subject{"User", RBACAPIGroup, "testuser3@test.domain", "ns1"},
			v1.Subject{"User", RBACAPIGroup, "testuser4@test.domain", "ns2"}}
		// should return true as slices have different length
		assert.True(t, hasDifferentSubjects(s1, s2))
		// should return false as match is found
		assert.False(t, hasDifferentSubjects(s2, s3))
		// should return true as no match is found
		assert.True(t, hasDifferentSubjects(s3, s4))
	})
}

func getMockSynchronizer() *Synchronizer {
	return NewSynchronizer(fake.NewSimpleClientset(), MockAdminService{}, time.Second*10, "testuser@test.domain", "testing", "", "")
}
