package main

import (
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/rbac/v1"
	"testing"
)

func TestRolebindings(t *testing.T) {
	t.Run("finds orphan role bindings", func(t *testing.T) {
		match := roleBinding("a", "ns1", "admin", nil)
		diff := roleBinding("b", "ns2", "admin", nil)
		desired := []v1.RoleBinding{match}
		actual := []v1.RoleBinding{match, diff}

		roleBindings := diff(desired, actual)
		assert.Equal(t, len(roleBindings), 1)
		assert.Equal(t, roleBindings[0], diff)
	})

	t.Run("finds updated role bindings when role has changed", func(t *testing.T) {
		aWithAdmin := roleBinding("a", "ns1", "admin", nil)
		aWithEdit := roleBinding("a", "ns1", "edit", nil)

		desired := []v1.RoleBinding{aWithAdmin}
		actual := []v1.RoleBinding{aWithEdit}
		toUpdate := roleBindingsToUpdate(desired, actual)

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
}
