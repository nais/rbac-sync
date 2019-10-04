package main

import (
	"github.com/stretchr/testify/assert"
	rbacv1 "k8s.io/api/rbac/v1"
	"testing"
)

func TestRolebindings(t *testing.T) {
	t.Run("finds orphan role bindings", func(t *testing.T) {
		r1 := roleBinding("a", "ns1", "admin", nil)
		r2 := roleBinding("b", "ns2", "admin", nil)

		roleBindings := diff([]rbacv1.RoleBinding{r1}, []rbacv1.RoleBinding{r1, r2})
		assert.Equal(t, len(roleBindings), 1)
		assert.Equal(t, roleBindings[0], r2)
	})

	t.Run("finds no role bindings when subjects are just out of order", func(t *testing.T) {
		r1 := roleBinding("a", "ns1", "admin", []string{"x", "y", "z"})
		r2 := roleBinding("a", "ns1", "admin", []string{"z", "x", "y"})

		toUpdate := roleBindingsToUpdate([]rbacv1.RoleBinding{r1}, []rbacv1.RoleBinding{r2})
		assert.Equal(t, len(toUpdate), 0)
	})

	t.Run("finds updated role bindings when subject has been added", func(t *testing.T) {
		r1 := roleBinding("a", "ns1", "admin", []string{"x", "y", "z"})
		r2 := roleBinding("a", "ns1", "admin", []string{"x", "y"})

		toUpdate := roleBindingsToUpdate([]rbacv1.RoleBinding{r1}, []rbacv1.RoleBinding{r2})
		assert.Equal(t, len(toUpdate), 1)
		assert.Equal(t, toUpdate[0], r1)
	})

	t.Run("finds updated role bindings when subject has been removed", func(t *testing.T) {
		r1 := roleBinding("a", "ns1", "admin", []string{"x", "y"})
		r2 := roleBinding("a", "ns1", "admin", []string{"x", "y", "z"})

		toUpdate := roleBindingsToUpdate([]rbacv1.RoleBinding{r1}, []rbacv1.RoleBinding{r2})
		assert.Equal(t, len(toUpdate), 1)
		assert.Equal(t, toUpdate[0], r1)
	})

	t.Run("finds updated role bindings when subject has changed", func(t *testing.T) {
		r1 := roleBinding("a", "ns1", "admin", []string{"x", "y", "z"})
		r2 := roleBinding("a", "ns1", "admin", []string{"a", "x", "y"})

		toUpdate := roleBindingsToUpdate([]rbacv1.RoleBinding{r1}, []rbacv1.RoleBinding{r2})
		assert.Equal(t, len(toUpdate), 1)
		assert.Equal(t, toUpdate[0], r1)
	})

	t.Run("errors when not finding any matching role bindings", func(t *testing.T) {
		roleBindings := []rbacv1.RoleBinding{roleBinding("a", "ns2", "", nil)}
		_, err := getMatchingRoleBinding(roleBinding("a", "ns1", "", nil), roleBindings)
		assert.Error(t, err)
	})

	t.Run("test subject diff evaluator", func(t *testing.T) {
		s1 := []rbacv1.Subject{{"User", RBACAPIGroup, "testuser@test.domain", "ns1"}}
		s2 := []rbacv1.Subject{{"User", RBACAPIGroup, "testuser@test.domain", "ns1"},
			{"User", RBACAPIGroup, "testuser2@test.domain", "ns2"}}
		s3 := []rbacv1.Subject{{"User", RBACAPIGroup, "testuser@test.domain", "ns1"},
			{"User", RBACAPIGroup, "testuser2@test.domain", "ns2"}}
		s4 := []rbacv1.Subject{{"User", RBACAPIGroup, "testuser3@test.domain", "ns1"},
			{"User", RBACAPIGroup, "testuser4@test.domain", "ns2"}}
		// should return true as slices have different length
		assert.True(t, hasDifferentSubjects(s1, s2))
		// should return false as match is found
		assert.False(t, hasDifferentSubjects(s2, s3))
		// should return true as no match is found
		assert.True(t, hasDifferentSubjects(s3, s4))
	})
}
