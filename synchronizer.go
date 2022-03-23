package main

import (
	"context"
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
	AnnotationNS                = "rbac-sync.nais.io"
	ManagedLabel                = AnnotationNS + "/managed"
	GroupNameAnnotation         = AnnotationNS + "/group-name"
	RolesAnnotation             = AnnotationNS + "/roles"
	RolebindingPrefixAnnotation = AnnotationNS + "/rolebinding-prefix"
	RBACAPIGroup                = "rbac.authorization.k8s.io"
)

type Synchronizer struct {
	Clientset                kubernetes.Interface
	IAMClient                IAMClient
	UpdateInterval           time.Duration
	GCPAdminUser             string
	ServiceAccountKeyFile    string
	DefaultRoles             string
	DefaultRoleBindingPrefix string
}

func NewSynchronizer(clientSet kubernetes.Interface,
	iamClient IAMClient,
	updateInterval time.Duration,
	gcpAdminUser string,
	serviceAccountKeyFile string,
	defaultRoleNames string,
	defaultRolebindingName string) *Synchronizer {
	return &Synchronizer{
		Clientset:                clientSet,
		IAMClient:                iamClient,
		UpdateInterval:           updateInterval,
		GCPAdminUser:             gcpAdminUser,
		ServiceAccountKeyFile:    serviceAccountKeyFile,
		DefaultRoles:             defaultRoleNames,
		DefaultRoleBindingPrefix: defaultRolebindingName,
	}
}

func (s Synchronizer) String() string {
	return fmt.Sprintf("update interval: %s, GCP admin user: %s, default roles: %s, default role binding prefix: %s",
		s.UpdateInterval, s.GCPAdminUser, s.DefaultRoles, s.DefaultRoleBindingPrefix)
}

// Read namespaces and synchronizes the desired state with the actual cluster state in duration intervals
func (s *Synchronizer) synchronizeRBAC() {
	ctx := context.Background()
	for {
		current, err := s.getCurrentManagedRoleBindings(ctx)
		if err != nil {
			continue
		}

		// Generate desired rolebindings based on namespace annotations
		desired := s.getDesiredRoleBindings(s.getTargetNamespaces(ctx))

		// Managed bindings that exist in cluster, but is not part of the configuration
		orphans := diff(desired, current)
		s.deleteRoleBindings(ctx, orphans)
		promSuccess.WithLabelValues("delete-orphan").Add(float64(len(orphans)))

		// Remove orphans from list of current role bindings
		current = diff(orphans, current)

		// New role bindings to create
		added := diff(current, desired)

		if err := s.createRoleBindings(ctx, added); err != nil {
			continue
		}

		promSuccess.WithLabelValues("create-rolebinding").Add(float64(len(added)))

		// Add newly created role bindings to list of current role bindings in the cluster
		current = append(current, added...)

		s.updateRoleBindings(ctx, roleBindingsToUpdate(desired, current))

		log.Debugf("sleeping for %s", s.UpdateInterval)
		time.Sleep(s.UpdateInterval)
	}
}

// Updates role binding by deleting and re-creating it because spec.roleRef.Name is immutable
func (s *Synchronizer) updateRoleBindings(ctx context.Context, roleBindings []v1.RoleBinding) {
	for _, roleBinding := range roleBindings {
		if err := s.deleteRoleBinding(ctx, roleBinding); err != nil {
			continue
		}

		if err := s.createRoleBinding(ctx, roleBinding); err != nil {
			continue
		}
	}

	promSuccess.WithLabelValues("updated-rolebinding").Add(float64(len(roleBindings)))
}

func (s *Synchronizer) createRoleBindings(ctx context.Context, roleBindings []v1.RoleBinding) error {
	for _, binding := range roleBindings {
		if err := s.createRoleBinding(ctx, binding); err != nil {
			return err
		}
	}
	return nil
}

func (s *Synchronizer) deleteRoleBindings(ctx context.Context, roleBindings []v1.RoleBinding) error {
	for _, binding := range roleBindings {
		if err := s.deleteRoleBinding(ctx, binding); err != nil {
			return err
		}
	}
	return nil
}

func (s *Synchronizer) deleteRoleBinding(ctx context.Context, roleBinding v1.RoleBinding) error {
	if err := s.Clientset.RbacV1().RoleBindings(roleBinding.Namespace).Delete(ctx, roleBinding.Name, metav1.DeleteOptions{}); err != nil {
		promErrors.WithLabelValues("delete-rolebinding").Inc()
		log.Errorf("unable to delete rolebinding %s in namespace %s: %s", roleBinding.Name, roleBinding.Namespace, err)
		return err
	}

	log.Debugf("deleted rolebinding: %s in namespace: %s", roleBinding.Name, roleBinding.Namespace)

	return nil
}

func (s *Synchronizer) createRoleBinding(ctx context.Context, binding v1.RoleBinding) error {
	_, err := s.Clientset.RbacV1().RoleBindings(binding.Namespace).Create(ctx, &binding, metav1.CreateOptions{})

	if err != nil {
		promErrors.WithLabelValues("create-rolebinding").Inc()
		log.Errorf("unable to create rolebinding %s in namespace %s: %s", binding.Name, binding.Namespace, err)
		return err
	}

	log.Debugf("created rolebinding: %s in namespace: %s", binding.Name, binding.Namespace)

	return nil
}

func (s *Synchronizer) getCurrentManagedRoleBindings(ctx context.Context) (roleBindings []v1.RoleBinding, err error) {
	bindingList, err := s.Clientset.RbacV1().RoleBindings("").List(ctx, metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=true", ManagedLabel)})

	if err != nil {
		promErrors.WithLabelValues("get-current-rolebindings").Inc()
		log.Error(err)
		return nil, fmt.Errorf("unable to get current managed rolebindings: %s", err)
	}

	return bindingList.Items, nil
}

func (s *Synchronizer) getDesiredRoleBindings(namespaces []corev1.Namespace) (rolebindings []v1.RoleBinding) {
	for _, ns := range namespaces {
		group := ns.Annotations[GroupNameAnnotation]
		members, err := s.IAMClient.getMembers(group)

		if err != nil {
			log.Errorf("unable to get members for group %s: %s", group, err)
			continue
		}

		rolebindingName := ensureVal(ns.Annotations[RolebindingPrefixAnnotation], s.DefaultRoleBindingPrefix)
		roleNames := ensureVal(ns.Annotations[RolesAnnotation], s.DefaultRoles)

		for _, role := range strings.Split(roleNames, ",") {
			rolebindings = append(rolebindings, roleBinding(rolebindingName, ns.Name, role, members))
		}
	}

	return
}

func (s *Synchronizer) getTargetNamespaces(ctx context.Context) (managedNamespaces []corev1.Namespace) {
	namespaces, err := s.Clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
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
