package main

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"strings"
	"time"
)

const (
	AnnotationNS              = "rbac-sync.nais.io"
	ManagedLabel              = AnnotationNS + "/managed"
	GroupNameAnnotation       = AnnotationNS + "/group-name"
	RoleNameAnnotation        = AnnotationNS + "/role-name"
	RolebindingNameAnnotation = AnnotationNS + "/rolebinding-name"
	RBACAPIGroup              = "rbac.authorization.k8s.io"
)

type Synchronizer struct {
	Clientset              kubernetes.Interface
	IAMClient              IAMClient
	UpdateInterval         time.Duration
	GCPAdminUser           string
	ServiceAccountKeyFile  string
	DefaultRoleName        string
	DefaultRoleBindingName string
}

func NewSynchronizer(clientSet kubernetes.Interface,
	iamClient IAMClient,
	updateInterval time.Duration,
	gcpAdminUser string,
	serviceAccountKeyFile string,
	defaultRoleName string,
	defaultRolebindingName string) *Synchronizer {
	return &Synchronizer{
		Clientset:              clientSet,
		IAMClient:              iamClient,
		UpdateInterval:         updateInterval,
		GCPAdminUser:           gcpAdminUser,
		ServiceAccountKeyFile:  serviceAccountKeyFile,
		DefaultRoleName:        defaultRoleName,
		DefaultRoleBindingName: defaultRolebindingName,
	}
}

func (s Synchronizer) String() string {
	return fmt.Sprintf("update interval: %s, GCP admin user: %s, default role name: %s, default role binding name: %s",
		s.UpdateInterval, s.GCPAdminUser, s.DefaultRoleName, s.DefaultRoleBindingName)
}

// Read namespaces and synchronizes the desired state with the actual cluster state in duration intervals
func (s *Synchronizer) synchronizeRBAC() {
	for {
		current, err := s.getCurrentManagedRoleBindings()
		if err != nil {
			continue
		}

		desired, err := s.getDesiredRoleBindings(s.getTargetNamespaces())
		if err != nil {
			log.Errorf("unable to get desired rolebindings", err)
		}

		// Managed bindings that exist in cluster, but is not part of the configuration
		orphans := diff(desired, current)
		s.deleteRoleBindings(orphans)
		promSuccess.WithLabelValues("delete-orphan").Add(float64(len(orphans)))

		// Remove orphans from list of current role bindings
		current = diff(orphans, current)

		// New role bindings to create
		added := diff(current, desired)

		if err := s.createRoleBindings(added); err != nil {
			continue
		}
		promSuccess.WithLabelValues("create-rolebinding").Add(float64(len(added)))

		// Add newly created role bindings to list of current role bindings in the cluster
		current = append(current, added...)

		s.updateRoleBindings(roleBindingsToUpdate(desired, current))

		log.Debugf("sleeping for %s", s.UpdateInterval)
		time.Sleep(s.UpdateInterval)
	}
}

// Updates role binding by deleting and re-creating it because spec.roleRef.Name is immutable
func (s *Synchronizer) updateRoleBindings(roleBindings []v1.RoleBinding) {
	for _, roleBinding := range roleBindings {
		if err := s.deleteRoleBinding(roleBinding); err != nil {
			continue
		}

		if err := s.createRoleBinding(roleBinding); err != nil {
			continue
		}
	}

	promSuccess.WithLabelValues("updated-rolebinding").Add(float64(len(roleBindings)))
}

func (s *Synchronizer) createRoleBindings(roleBindings []v1.RoleBinding) error {
	for _, binding := range roleBindings {
		if err := s.createRoleBinding(binding); err != nil {
			return err
		}
	}
	return nil
}

func (s *Synchronizer) deleteRoleBindings(roleBindings []v1.RoleBinding) error {
	for _, binding := range roleBindings {
		if err := s.deleteRoleBinding(binding); err != nil {
			return err
		}
	}
	return nil
}

func (s *Synchronizer) deleteRoleBinding(roleBinding v1.RoleBinding) error {
	if err := s.Clientset.RbacV1().RoleBindings(roleBinding.Namespace).Delete(roleBinding.Name, nil); err != nil {
		promErrors.WithLabelValues("delete-rolebinding").Inc()
		log.Errorf("unable to delete rolebinding %s in namespace %s: %s", roleBinding.Name, roleBinding.Namespace, err)
		return err
	}

	log.Debugf("deleted rolebinding: %s in namespace: %s", roleBinding.Name, roleBinding.Namespace)

	return nil
}

func (s *Synchronizer) createRoleBinding(binding v1.RoleBinding) error {
	_, err := s.Clientset.RbacV1().RoleBindings(binding.Namespace).Create(&binding)

	if err != nil {
		promErrors.WithLabelValues("create-rolebinding").Inc()
		log.Errorf("unable to create rolebinding %s in namespace %s: %s", binding.Name, binding.Namespace, err)
		return err
	}

	log.Debugf("created rolebinding: %s in namespace: %s", binding.Name, binding.Namespace)

	return nil
}

func roleBindingsToUpdate(desired []v1.RoleBinding, current []v1.RoleBinding) (updated []v1.RoleBinding) {
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

func getMatchingRoleBinding(roleBinding v1.RoleBinding, roleBindings []v1.RoleBinding) (*v1.RoleBinding, error) {
	for _, rb := range roleBindings {
		if roleBinding.Name == rb.Name && roleBinding.Namespace == rb.Namespace {
			return &rb, nil
		}

	}
	return nil, fmt.Errorf("unable to find matching rolebinding, this is bad")
}

func hasDifferentSubjects(s1 []v1.Subject, s2 []v1.Subject) bool {
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
func diff(base, roleBindings []v1.RoleBinding) (diff []v1.RoleBinding) {
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

func (s *Synchronizer) getCurrentManagedRoleBindings() (roleBindings []v1.RoleBinding, err error) {
	bindingList, err := s.Clientset.RbacV1().RoleBindings("").List(metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=true", ManagedLabel)})

	if err != nil {
		promErrors.WithLabelValues("get-current-rolebindings").Inc()
		log.Error(err)
		return nil, fmt.Errorf("unable to get current managed rolebindings: %s", err)
	}

	return bindingList.Items, nil
}

func (s *Synchronizer) getDesiredRoleBindings(namespaces []corev1.Namespace) (rolebindings []v1.RoleBinding, err error) {
	for _, ns := range namespaces {
		group := ns.Annotations[GroupNameAnnotation]
		members, err := s.IAMClient.getMembers(group)

		if err != nil {
			return nil, fmt.Errorf("unable to get members for group %s: %s", group, err)
		}

		rolebindingName := ensureVal(ns.Annotations[RolebindingNameAnnotation], s.DefaultRoleBindingName)
		roleName := ensureVal(ns.Annotations[RoleNameAnnotation], s.DefaultRoleName)
		rolebindings = append(rolebindings, roleBinding(rolebindingName, ns.Name, roleName, members))
	}

	return
}

func roleBinding(name string, namespace string, role string, members []string) v1.RoleBinding {
	return v1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				ManagedLabel: "true",
			}},
		RoleRef: v1.RoleRef{
			Kind:     "Role",
			APIGroup: RBACAPIGroup,
			Name:     role,
		},
		Subjects: subjects(members),
	}
}

func subjects(members []string) (subjects []v1.Subject) {
	for _, member := range members {

		subjects = append(subjects, v1.Subject{
			Kind:     "User",
			APIGroup: RBACAPIGroup,
			Name:     member,
		})
	}

	return
}

func (s *Synchronizer) getTargetNamespaces() (managedNamespaces []corev1.Namespace) {
	namespaces, err := s.Clientset.CoreV1().Namespaces().List(metav1.ListOptions{})
	if err != nil {
		log.Errorf("unable to get all namespaces: %s", err)
	}

	for _, namespace := range namespaces.Items {
		if len(namespace.Annotations[GroupNameAnnotation]) > 0 {
			managedNamespaces = append(managedNamespaces, namespace)
		}
	}

	return
}

func ensureVal(val string, fallback string) string {
	if len(strings.TrimSpace(val)) > 0 {
		return val
	}

	return fallback
}
