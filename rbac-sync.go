package main

import (
	"context"
	"flag"
	"io/ioutil"
	v1 "k8s.io/api/core/v1"
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

var (
	promSuccess = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rbac_sync_success",
			Help: "Cumulative number of role update operations",
		},
		[]string{"count"},
	)

	promErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rbac_sync_errors",
			Help: "Cumulative number of errors during role update operations",
		},
		[]string{"count"},
	)
)

var serviceAccountKeyFile string
var gcpAdminUser string
var updateInterval time.Duration
var roleName, roleBindingName, bindAddress string

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
		log.WithFields(log.Fields{
			"error": error,
		}).Error("Unable to get kubernetes client.")
		return
	}

	namespaces := getAllNamespaces(clientSet)

	for _, namespace := range namespaces.Items {
		namespaceName := namespace.Name
		groupName := namespace.Annotations["rbac-sync.nais.io/group-name"]
		if namespace.Annotations["rbac-sync.nais.io/role-name"] == "" {
			roleName = "nais:developer"
		}

		if namespace.Annotations["rbac-sync.nais.io/rolebinding-name"] == "" {
			roleName = "teammember"
		}

		updateRoles(namespaceName, groupName, roleName, clientSet)
	}

}

// Get all namespaces
func getAllNamespaces (clientset *kubernetes.Clientset) *v1.NamespaceList {
	api := clientset.CoreV1()
	namespacesList, err := api.Namespaces().List(metav1.ListOptions{FieldSelector: "metadata.annotations.rbac-sync.nais.io/group-name"})
	if err != nil {
		log.WithFields(log.Fields{
		"error": err,
		}).Error("Unable to get namespace list.")
	}

	return namespacesList
}

// Handles SIGTERM and exits
func handleSigterm(stopChan chan struct{}) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM)
	<-signals
	log.Info("Received SIGTERM. Terminating...")
	close(stopChan)
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

	log.WithFields(log.Fields{
		"address": address,
	}).Info("Server started")
	log.Fatal(http.ListenAndServe(address, nil))
}

// Gets group users and updates kubernetes rolebindings
func updateRoles(namespaceName, groupName, roleName string, clientSet *kubernetes.Clientset) {
	service := getService(serviceAccountKeyFile, gcpAdminUser)
	
	result, error := getMembers(service, groupName)
	if error != nil {
		log.WithFields(log.Fields{
			"error": error,
		}).Error("Unable to get members.")
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
			Name:      roleBindingName,
			Namespace: namespaceName,
			Annotations: map[string]string{
				"lastSync": time.Now().UTC().Format(time.RFC3339),
			},
		},
		RoleRef: rbacv1beta1.RoleRef{
			Kind:     "Role",
			APIGroup: "rbac.authorization.k8s.io",
			Name:     roleName,
		},
		Subjects: subjects,
	}

	roleClient := clientSet.RbacV1beta1().RoleBindings(namespaceName)
	updateResult, updateError := roleClient.Update(roleBinding)
	if updateError != nil {
		promErrors.WithLabelValues("role-update").Inc()
		log.WithFields(log.Fields{
			"rolebinding": roleBindingName,
			"error":       updateError,
		}).Error("Unable to update rolebinding.")
		return
	}
	log.WithFields(log.Fields{
		"rolebinding": updateResult.GetObjectMeta().GetName(),
		"namespace":   namespaceName,
	}).Info("Updated rolebinding.")
	promSuccess.WithLabelValues("role-update").Inc()
}

// Build and returns an Admin SDK Directory service object authorized with
// the service accounts that act on behalf of the given user.
func getService(serviceAccountKeyfile string, gcpAdminUser string) *admin.Service {
	jsonCredentials, err := ioutil.ReadFile(serviceAccountKeyfile)
	if err != nil {
		promErrors.WithLabelValues("get-serviceaccount-keyfile").Inc()
		log.WithFields(log.Fields{
			"error": err,
		}).Error("Unable to read service account key file.")
		return nil
	}

	config, err := google.JWTConfigFromJSON(jsonCredentials, admin.AdminDirectoryGroupMemberReadonlyScope, admin.AdminDirectoryGroupReadonlyScope)
	if err != nil {
		promErrors.WithLabelValues("get-serviceaccount-secret").Inc()
		log.WithFields(log.Fields{
			"error": err,
		}).Error("Unable to parse service account key file to config.")
		return nil
	}
	config.Subject = gcpAdminUser
	ctx := context.Background()
	client := config.Client(ctx)

	service, err := admin.New(client)
	if err != nil {
		promErrors.WithLabelValues("get-kube-client").Inc()
		log.WithFields(log.Fields{
			"error": err,
		}).Error("Unable to retrieve Google Admin Client.")
		return nil
	}
	return service
}

// Gets kubernetes config and client
func getKubeClient() (*kubernetes.Clientset, error) {
	var kubeClusterConfig *rest.Config

	inClusterConfig, err := rest.InClusterConfig()
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Error("Unable to get in kubernetes cluster config.")
	}

	kubeClusterConfig = inClusterConfig

	clientSet, err := kubernetes.NewForConfig(kubeClusterConfig)
	if err != nil {
		promErrors.WithLabelValues("get-kube-client").Inc()
		log.WithFields(log.Fields{
			"error": err,
		}).Error("Unable to get kube client.")
	}

	return clientSet, err
}

// Gets recursive the group members by e-mail address
func getMembers(service *admin.Service, groupname string) ([]*admin.Member, error) {
	result, err := service.Members.List(groupname).Do()
	if err != nil {
		promErrors.WithLabelValues("get-members").Inc()
		log.WithFields(log.Fields{
			"error": err,
		}).Error("Unable to get group members.")
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
