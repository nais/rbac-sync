package main

import (
	"fmt"
	"github.com/prometheus/common/log"
	corev1 "k8s.io/api/core/v1"
	rbacv1beta1 "k8s.io/api/rbac/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/typed/rbac/v1beta1"
	"time"
)

const (
	AnnotationNS              = "rbac-sync.nais.io"
	ManagedAnnotation         = AnnotationNS + "/managed"
	GroupNameAnnotation       = AnnotationNS + "/group-name"
	RoleNameAnnotation        = AnnotationNS + "/role-name"
	RolebindingNameAnnotation = AnnotationNS + "/rolebinding-name"
)

type Synchronizer struct {
	Clientset              *kubernetes.Clientset
	UpdateInterval         time.Duration
	GCPAdminUser           string
	ServiceAccountKeyFile  string
	DefaultRoleName        string
	DefaultRolebindingName string
}

// RbacConfiguration is a struct to hold information necessary for role binding creation
type RbacConfiguration struct {
	namespace       string
	groupName       string
	roleName        string
	rolebindingName string
}

func NewSynchronizer(clientSet *kubernetes.Clientset,
	updateInterval time.Duration,
	gcpAdminUser string,
	serviceAccountKeyFile string,
	defaultRoleName string,
	defaultRolebindingName string) *Synchronizer {
	return &Synchronizer{
		Clientset:              clientSet,
		UpdateInterval:         updateInterval,
		GCPAdminUser:           gcpAdminUser,
		ServiceAccountKeyFile:  serviceAccountKeyFile,
		DefaultRoleName:        defaultRoleName,
		DefaultRolebindingName: defaultRolebindingName,
	}
}

// Read namespaces and update roles in duration intervals
// Uses the clientset to fetch namespaces and update the rolebindings
func (s *Synchronizer) synchronizeRBAC() {
	for {
		for _, namespace := range s.getAllNamespaces() {
			// Only configure Rolebinding if group name annotation is set
			if len(namespace.Annotations[GroupNameAnnotation]) > 0 {
				if err := s.configureRoleBinding(namespace); err != nil {
					promErrors.WithLabelValues("sync_error").Inc()
					log.Error(err)
				}
			}
		}

		log.Infof("Sleeping for %s", s.UpdateInterval)
		time.Sleep(s.UpdateInterval)
	}
}

func (s *Synchronizer) NewRbacConfiguration(namespace corev1.Namespace) *RbacConfiguration {
	cfg := &RbacConfiguration{
		namespace:       namespace.Name,
		groupName:       namespace.Annotations[GroupNameAnnotation],
		roleName:        namespace.Annotations[RoleNameAnnotation],
		rolebindingName: namespace.Annotations[RolebindingNameAnnotation],
	}

	if len(cfg.roleName) == 0 {
		cfg.roleName = s.DefaultRoleName
	}

	if len(cfg.rolebindingName) == 0 {
		cfg.rolebindingName = s.DefaultRolebindingName
	}

	return cfg
}

func (s *Synchronizer) configureRoleBinding(namespace corev1.Namespace) error {
	roleClient := s.Clientset.RbacV1beta1().RoleBindings(namespace.Name)

	// Delete the roles in each namespace so we also delete role bindings
	// in namespaces that have removed annotations on namespace
	if err := deleteRoleBindingsInNamespace(roleClient); err != nil {
		return fmt.Errorf("Unable to delete role bindings: %s", err)
	}

	rbacConfiguration := s.NewRbacConfiguration(namespace)

	return s.updateRoles(roleClient, rbacConfiguration)
}

func (s *Synchronizer) getAllNamespaces() []corev1.Namespace {
	namespacesList, err := s.Clientset.CoreV1().Namespaces().List(metav1.ListOptions{})
	if err != nil {
		log.Errorf("Unable to get all namespaces: %s", err)
	}

	return namespacesList.Items
}

// Gets group users and updates kubernetes rolebindings
func (s *Synchronizer) updateRoles(roleClient v1beta1.RoleBindingInterface, configuration *RbacConfiguration) error {
	service := getAdminService(s.ServiceAccountKeyFile, s.GCPAdminUser)

	result, error := getMembers(service, configuration.groupName)
	if error != nil {
		return fmt.Errorf("unable to get members: %s", error)
	}

	var subjects []rbacv1beta1.Subject
	for _, member := range uniq(result) {
		subjects = append(subjects, getSubjectWithEmail(member.Email))
	}

	roleBinding := getRoleBindingWithSubjects(configuration, subjects)

	updateResult, updateError := roleClient.Create(&roleBinding)
	if updateError != nil {
		promErrors.WithLabelValues("role-update").Inc()
		return fmt.Errorf("unable to update rolebinding %s: %s", configuration.rolebindingName, updateError)
	}

	log.Infof("Updated rolebinding %s in %s", updateResult.GetObjectMeta().GetName(), configuration.namespace)

	return nil
}

// Deletes all rolebindings managed by rbac-sync in the namespace
func deleteRoleBindingsInNamespace(roleClient v1beta1.RoleBindingInterface) error {
	rolebindings, err := roleClient.List(metav1.ListOptions{})
	if err != nil {
		log.Errorf("Unable to get role bindings list: %s", err)
		return err
	}
	for _, rolebinding := range rolebindings.Items {
		if rolebinding.Annotations[ManagedAnnotation] == "true" {
			log.Infof("Deleting role binding %s in %s", rolebinding.Name, rolebinding.GetObjectMeta().GetNamespace())
			if err := roleClient.Delete(rolebinding.Name, &metav1.DeleteOptions{}); err != nil {
				return fmt.Errorf("Unable to delete rolebinding: %s, in namespace: %s", rolebinding.Name, rolebinding.GetObjectMeta().GetNamespace())
			}
		}
	}
	return nil
}

func getRoleBindingWithSubjects(configuration *RbacConfiguration, subjects []rbacv1beta1.Subject) rbacv1beta1.RoleBinding {
	return rbacv1beta1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configuration.rolebindingName,
			Namespace: configuration.namespace,
			Annotations: map[string]string{
				ManagedAnnotation: "true",
			},
		},
		RoleRef: rbacv1beta1.RoleRef{
			Kind:     "Role",
			APIGroup: "rbac.authorization.k8s.io",
			Name:     configuration.roleName,
		},
		Subjects: subjects,
	}
}

func getSubjectWithEmail(email string) rbacv1beta1.Subject {
	return rbacv1beta1.Subject{
		Kind:     "User",
		APIGroup: "rbac.authorization.k8s.io",
		Name:     email,
	}
}
