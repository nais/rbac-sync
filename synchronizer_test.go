package main

import (
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"testing"
)

var s = NewSynchronizer(fake.NewSimpleClientset(), MockAdminService{}, time.Second*10, "testuser@test.domain", "testing", "", "")

func TestSynchronizer(t *testing.T) {

	t.Run("creates new role bindings", func(t *testing.T) {
		rolebindings := []rbacv1.RoleBinding{roleBinding("a", "ns1", "admin", []string{"x", "y", "z"}),
			roleBinding("b", "ns2", "admin", []string{"x", "y", "z"})}

		err := s.createRoleBindings(rolebindings)
		assert.NoError(t, err)
	})

	t.Run("error when creating identical role bindings", func(t *testing.T) {
		rolebindingsWithError := []rbacv1.RoleBinding{roleBinding("a", "ns1", "admin", []string{"x", "y", "z"}),
			roleBinding("a", "ns1", "admin", []string{"x", "y", "z"})}

		error := s.createRoleBindings(rolebindingsWithError)
		assert.Error(t, error)
	})

	t.Run("skips non-existent groups", func(t *testing.T) {
		rb := s.getDesiredRoleBindings([]corev1.Namespace{{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{"rbac-sync.nais.io/group-name": "nonexistent"},
			}}, {
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{"rbac-sync.nais.io/group-name": "foo@acme.no"},
			}}})

		assert.Equal(t, len(rb), 1, "contains single rolebinding for working ns")
	})

	t.Run("creates multiple rolebindings when multiple roles are requested", func(t *testing.T) {
		rbs := s.getDesiredRoleBindings([]corev1.Namespace{{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{GroupNameAnnotation: "foo@bar.com", RolesAnnotation: "a,b", RolebindingPrefixAnnotation: "prefix"},
			}}})

		assert.Len(t, rbs, 2)
		assert.Equal(t, rbs[0].Name, "prefix-a")
		assert.Equal(t, rbs[1].Name, "prefix-b")
	})
}
