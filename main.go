package main

import (
	"flag"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

var (
	kubeconfig             string
	serviceAccountKeyFile  string
	gcpAdminUser           string
	updateInterval         time.Duration
	bindAddress            string
	defaultRoleName        string
	defaultRolebindingName string
	mockIAM                bool
	debug                  bool
	promSuccess            = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      "successes",
			Namespace: "rbac_sync",
			Help:      "Cumulative number of successful operations"},
		[]string{"count"},
	)
	promErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      "errors",
			Namespace: "rbac_sync",
			Help:      "Cumulative number of failed operations"},
		[]string{"count"},
	)
)

func main() {
	flag.StringVar(&kubeconfig, "kubeconfig", "", "path to Kubernetes config file")
	flag.StringVar(&serviceAccountKeyFile, "serviceaccount-keyfile", "", "The path to the service account private key file.")
	flag.StringVar(&gcpAdminUser, "gcp-admin-user", "", "The google admin user e-mail address.")
	flag.StringVar(&bindAddress, "bind-address", ":8080", "Bind address for application.")
	flag.DurationVar(&updateInterval, "update-interval", time.Minute*5, "Update interval in seconds.")
	flag.StringVar(&defaultRoleName, "default-role-name", "rbacsync-default", "Default name for role if not specified in namespace annotation")
	flag.StringVar(&defaultRolebindingName, "default-rolebinding-name", "rbacsync-default", "Default name for rolebinding if not specified in namespace annotation")
	flag.BoolVar(&mockIAM, "mock-iam", false, "starts rbac-sync with a mocked version of the IAM client")
	flag.BoolVar(&debug, "debug", false, "enables debug logging")

	flag.Parse()

	setupLogging()

	if !mockIAM {
		if serviceAccountKeyFile == "" {
			flag.Usage()
			log.Fatal("missing configuration: -serviceaccount-keyfile")
		}
		if gcpAdminUser == "" {
			flag.Usage()
			log.Fatal("missing configuration: -gcp-admin-user")
		}
	}

	stopChan := make(chan struct{}, 1)

	go serve(bindAddress)
	go handleSigterm(stopChan)

	clientSet, error := getKubeClient()
	if error != nil {
		log.Fatalf("unable to get kubernetes client: %s", error)
	}

	var iamClient IAMClient
	if mockIAM {
		iamClient = MockAdminService{}
	} else {
		iamClient, error = NewAdminService(serviceAccountKeyFile, gcpAdminUser)
		if error != nil {
			log.Fatal(error)
		}
	}

	s := NewSynchronizer(clientSet, iamClient, updateInterval, gcpAdminUser, serviceAccountKeyFile, defaultRoleName, defaultRolebindingName)
	log.Infof("starting RBAC synchronizer: %s", s)
	s.synchronizeRBAC()
}

func setupLogging() {
	log.SetFormatter(&log.JSONFormatter{
		TimestampFormat: time.RFC3339Nano,
	})
	log.SetOutput(os.Stdout)
	if debug {
		log.SetLevel(log.DebugLevel)
	}
}

// Provides health check and metrics routes
func serve(address string) {
	http.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	prometheus.MustRegister(promErrors)

	http.Handle("/metrics", promhttp.Handler())

	log.Infof("server started on %s", address)
	log.Fatal(http.ListenAndServe(address, nil))
}

// Handles SIGTERM and exits
func handleSigterm(stopChan chan struct{}) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM)
	<-signals
	log.Info("received SIGTERM. Terminating...")
	close(stopChan)
}

// Gets kubernetes config and client
func getKubeClient() (*kubernetes.Clientset, error) {
	kubeconfig, err := getK8sConfig()
	if err != nil {
		log.Fatal("unable to initialize kubernetes config")
	}

	clientSet, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		log.Errorf("unable to get kube client: %s", err)
	}

	return clientSet, err
}

func getK8sConfig() (*rest.Config, error) {
	if kubeconfig == "" {
		log.Infof("using in-cluster configuration")
		return rest.InClusterConfig()
	} else {
		log.Infof("using configuration from '%s'", kubeconfig)
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
}
