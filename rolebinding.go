package main

import (
	"fmt"
	"github.com/prometheus/common/log"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func roleBindingsToUpdate(desired []rbacv1.RoleBinding, current []rbacv1.RoleBinding) (updated []rbacv1.RoleBinding) {
	for _, rolebinding := range desired {
		match, err := getMatchingRoleBinding(rolebinding, current)
		if err != nil {
			promErrors.WithLabelValues("no-matching-rolebinding").Inc()
			log.Error(err)
		}

		if rolebinding.RoleRef.Name != match.RoleRef.Name {
			updated = append(updated, rolebinding)
			continue
		}

		if hasDifferentSubjects(rolebinding.Subjects, match.Subjects) {
			updated = append(updated, rolebinding)
			continue
		}
	}

	return
}

func getMatchingRoleBinding(roleBinding rbacv1.RoleBinding, roleBindings []rbacv1.RoleBinding) (*rbacv1.RoleBinding, error) {
	for _, rb := range roleBindings {
		if roleBinding.Name == rb.Name && roleBinding.Namespace == rb.Namespace {
			return &rb, nil
		}

	}
	return nil, fmt.Errorf("unable to find matching rolebinding, this is bad")
}

// hasDifferentSubjects checks compares two slices of Subjects and returns true
// if they contain different members
func hasDifferentSubjects(s1 []rbacv1.Subject, s2 []rbacv1.Subject) bool {
	if len(s1) != len(s2) {
		return true
	}

	for _, subject1 := range s1 {
		match := false
		for _, subject2 := range s2 {
			if subject1.Name == subject2.Name {
				match = true
			}
		}

		if !match {
			return true
		}
	}
	return false
}

// returns the difference between two slices of rolebinding objects as a new slice
func diff(base, roleBindings []rbacv1.RoleBinding) (diff []rbacv1.RoleBinding) {
	for _, roleBinding := range roleBindings {
		match := false
		for _, baseRoleBinding := range base {
			if baseRoleBinding.Name == roleBinding.Name && baseRoleBinding.Namespace == roleBinding.Namespace {
				match = true
			}
		}

		if !match {
			diff = append(diff, roleBinding)
		}
	}

	return
}

func roleBinding(rolebindingPrefix string, namespace string, role string, members []string) rbacv1.RoleBinding {
	return rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", rolebindingPrefix, role),
			Namespace: namespace,
			Labels: map[string]string{
				ManagedLabel: "true",
			}},
		RoleRef: rbacv1.RoleRef{
			Kind:     "ClusterRole",
			APIGroup: RBACAPIGroup,
			Name:     role,
		},
		Subjects: subjects(members),
	}
}

func subjects(members []string) (subjects []rbacv1.Subject) {
	for _, member := range members {

		subjects = append(subjects, rbacv1.Subject{
			Kind:     "User",
			APIGroup: RBACAPIGroup,
			Name:     member,
		})
	}

	return
}
