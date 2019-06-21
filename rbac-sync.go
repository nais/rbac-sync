package main

import (
	"context"
	"flag"
	"io/ioutil"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/typed/rbac/v1beta1"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/admin/directory/v1"
	rbacv1beta1 "k8s.io/api/rbac/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Using struct to hold information necessary for role binding creation for future use
type RbacConfiguration struct {
	namespace       string
	groupname       string
	rolename        string
	rolebindingname string
}

var (
	promSuccess = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "rbac_sync_success", Help: "Cumulative number of role update operations"},
		[]string{"count"},
	)

	promErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rbac_sync_errors", Help: "Cumulative number of errors during role update operations"},
		[]string{"count"},
	)
)

var serviceAccountKeyFile string
var gcpAdminUser string
var updateInterval time.Duration
var bindAddress string

// Returns populated RbacConfiguration (for future use)
func NewRbacConfiguration(namespace, groupname, rolename, rolebindingname string) *RbacConfiguration {
	return &RbacConfiguration{
		namespace:       namespace,
		groupname:       groupname,
		rolename:        rolename,
		rolebindingname: rolebindingname,
	}
}

func main() {
	flag.StringVar(&serviceAccountKeyFile, "serviceaccount-keyfile", "", "The Path to the Service Account Private Key file.")
	flag.StringVar(&gcpAdminUser, "gcp-admin-user", "", "The google admin user e-mail address.")
	flag.StringVar(&bindAddress, "bind-address", ":8080", "Bind address for application.")
	flag.DurationVar(&updateInterval, "update-interval", time.Minute*5, "Update interval in seconds.")
	flag.Parse()

	log.SetOutput(os.Stdout)

	if serviceAccountKeyFile == "" {
		flag.Usage()
		log.Fatal("Missing -serviceaccount-keyfile")
	}
	if gcpAdminUser == "" {
		flag.Usage()
		log.Fatal("Missing -gcp-admin-user")
	}

	stopChan := make(chan struct{}, 1)

	go serveMetrics(bindAddress)
	go handleSigterm(stopChan)

	clientSet, error := getKubeClient()
	if error != nil {
		log.Errorf("Unable to get kubernetes client: %s", error)
		return
	}

	handleRoleBindings(clientSet, updateInterval)

}

// Provides health check and metrics routes
func serveMetrics(address string) {
	http.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	prometheus.MustRegister(promSuccess)
	prometheus.MustRegister(promErrors)
	http.Handle("/metrics", promhttp.Handler())

	log.Infof("Server started on %s", address)
	log.Fatal(http.ListenAndServe(address, nil))
}

// Handles SIGTERM and exits
func handleSigterm(stopChan chan struct{}) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM)
	<-signals
	log.Info("Received SIGTERM. Terminating...")
	close(stopChan)
}

// Read namespaces and update roles in duration intervals
// Uses the clientset to fetch namespaces and update the rolebindings
func handleRoleBindings(clientset *kubernetes.Clientset, updateInterval time.Duration) {
	for {
		namespaces := getAllNamespaces(clientset)

		for _, namespace := range namespaces.Items {
			roleClient := clientset.RbacV1beta1().RoleBindings(namespace.Name)

			// Delete the roles in each namespace so we also delete role bindings
			// in namespaces that have removed annotations on namespace
			err := deleteRoleBindingsInNamespace(roleClient)
			if err != nil {
				log.Errorf("Unable to delete role bindings: %s", err)
			}

			groupName := namespace.Annotations["rbac-sync.nais.io/group-name"]
			roleName := "nais:developer"
			roleBindingName := "teammember"
			if groupName != "" {
				if namespace.Annotations["rbac-sync.nais.io/role-name"] != "" {
					roleName = namespace.Annotations["rbac-sync.nais.io/role-name"]
				}

				if namespace.Annotations["rbac-sync.nais.io/rolebinding-name"] != "" {
					roleBindingName = namespace.Annotations["rbac-sync.nais.io/rolebinding-name"]
				}

				rbacConfiguration := NewRbacConfiguration(namespace.Name, groupName, roleName, roleBindingName)
				updateRoles(roleClient, rbacConfiguration)
			}
		}
		log.Infof("Sleeping for %s", updateInterval)
		time.Sleep(updateInterval)
	}
}

// Get all namespaces
func getAllNamespaces(clientset *kubernetes.Clientset) *v1.NamespaceList {
	api := clientset.CoreV1()
	namespacesList, err := api.Namespaces().List(metav1.ListOptions{})
	if err != nil {
		log.Errorf("Unable to get namespace list: %s", err)
	}

	return namespacesList
}

// Gets group users and updates kubernetes rolebindings
func updateRoles(roleClient v1beta1.RoleBindingInterface, configuration *RbacConfiguration) {
	service := getService(serviceAccountKeyFile, gcpAdminUser)

	result, error := getMembers(service, configuration.groupname)
	if error != nil {
		log.Errorf("Unable to get members: %s", error)
		return
	}

	var subjects []rbacv1beta1.Subject
	for _, member := range uniq(result) {
		subjects = append(subjects, rbacv1beta1.Subject{
			Kind:     "User",
			APIGroup: "rbac.authorization.k8s.io",
			Name:     member.Email,
		})
	}
	roleBinding := &rbacv1beta1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configuration.rolebindingname,
			Namespace: configuration.namespace,
			Annotations: map[string]string{
				"rbac-sync.nais.io/managed": "true",
			},
		},
		RoleRef: rbacv1beta1.RoleRef{
			Kind:     "Role",
			APIGroup: "rbac.authorization.k8s.io",
			Name:     configuration.rolename,
		},
		Subjects: subjects,
	}

	updateResult, updateError := roleClient.Create(roleBinding)
	if updateError != nil {
		promErrors.WithLabelValues("role-update").Inc()
		log.Errorf("Unable to update rolebinding %s: %s", configuration.rolebindingname, updateError)
		return
	}
	log.Infof("Updated rolebinding %s in %s", updateResult.GetObjectMeta().GetName(), configuration.namespace)
	promSuccess.WithLabelValues("role-update").Inc()
}

// Deletes all rolebindings managed by rbac-sync in the namespace
func deleteRoleBindingsInNamespace(roleClient v1beta1.RoleBindingInterface) error {
	rolebindings, err := roleClient.List(metav1.ListOptions{})
	if err != nil {
		log.Errorf("Unable to get role bindings list: %s", err)
		return err
	}
	for _, rolebinding := range rolebindings.Items {
		if rolebinding.Annotations["rbac-sync.nais.io/managed"] == "true" {
			log.Infof("Deleting role binding %s in %s", rolebinding.Name, rolebinding.GetObjectMeta().GetNamespace())
			roleClient.Delete(rolebinding.Name, &metav1.DeleteOptions{})
		}
	}
	return nil
}

// Build and returns an Admin SDK Directory service object authorized with
// the service accounts that act on behalf of the given user.
func getService(serviceAccountKeyfile string, gcpAdminUser string) *admin.Service {
	jsonCredentials, err := ioutil.ReadFile(serviceAccountKeyfile)
	if err != nil {
		promErrors.WithLabelValues("get-serviceaccount-keyfile").Inc()
		log.Errorf("Unable to read service account key file %s", err)
		return nil
	}

	config, err := google.JWTConfigFromJSON(jsonCredentials, admin.AdminDirectoryGroupMemberReadonlyScope, admin.AdminDirectoryGroupReadonlyScope)
	if err != nil {
		promErrors.WithLabelValues("get-serviceaccount-secret").Inc()
		log.Errorf("Unable to parse service account key file to config: %s", err)
		return nil
	}
	config.Subject = gcpAdminUser
	ctx := context.Background()
	client := config.Client(ctx)

	service, err := admin.New(client)
	if err != nil {
		promErrors.WithLabelValues("get-kube-client").Inc()
		log.Errorf("Unable to retrieve Google Admin Client: %s", err)
		return nil
	}
	return service
}

// Gets kubernetes config and client
func getKubeClient() (*kubernetes.Clientset, error) {
	var kubeClusterConfig *rest.Config

	inClusterConfig, err := rest.InClusterConfig()
	if err != nil {
		log.Errorf("Unable to get in kubernetes cluster config: %s", err)
	}

	kubeClusterConfig = inClusterConfig

	clientSet, err := kubernetes.NewForConfig(kubeClusterConfig)
	if err != nil {
		promErrors.WithLabelValues("get-kube-client").Inc()
		log.Errorf("Unable to get kube client: %s", err)
	}

	return clientSet, err
}

// Gets recursive the group members by e-mail address
func getMembers(service *admin.Service, groupname string) ([]*admin.Member, error) {
	result, err := service.Members.List(groupname).Do()
	if err != nil {
		promErrors.WithLabelValues("get-members").Inc()
		return nil, err
	}

	var userList []*admin.Member
	for _, member := range result.Members {
		if member.Type == "GROUP" {
			groupMembers, _ := getMembers(service, member.Email)
			userList = append(userList, groupMembers...)
		} else {
			userList = append(userList, member)
		}
	}

	return userList, nil
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
