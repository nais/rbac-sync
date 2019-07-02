package main

import (
	"flag"
	"k8s.io/client-go/kubernetes"
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
	promErrors             = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      "rbac_sync_errors",
			Namespace: "rbac_sync",
			Help:      "Cumulative number of errors during role update operations"},
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

	flag.Parse()

	log.SetFormatter(&log.JSONFormatter{
		TimestampFormat: time.RFC3339Nano,
	})

	log.SetOutput(os.Stdout)

	if serviceAccountKeyFile == "" {
		flag.Usage()
		log.Fatal("Missing configuration: -serviceaccount-keyfile")
	}
	if gcpAdminUser == "" {
		flag.Usage()
		log.Fatal("Missing configuration: -gcp-admin-user")
	}

	stopChan := make(chan struct{}, 1)

	go serveMetrics(bindAddress)
	go handleSigterm(stopChan)

	clientSet, error := getKubeClient()
	if error != nil {
		log.Errorf("Unable to get kubernetes client: %s", error)
		return
	}

	s := NewSynchronizer(clientSet, updateInterval, gcpAdminUser, serviceAccountKeyFile, defaultRoleName, defaultRolebindingName)
	s.synchronizeRBAC()
}

// Provides health check and metrics routes
func serveMetrics(address string) {
	http.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

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

// Gets kubernetes config and client
func getKubeClient() (*kubernetes.Clientset, error) {
	kubeconfig, err := getK8sConfig()
	if err != nil {
		log.Fatal("unable to initialize kubernetes config")
	}

	clientSet, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		log.Errorf("Unable to get kube client: %s", err)
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
