package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strconv"

	//promv1 "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"
	//ocpconfigv1 "github.com/openshift/api/config/v1"
	//routev1 "github.com/openshift/api/route/v1"
	//secv1 "github.com/openshift/api/security/v1"
	//apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	"go.uber.org/zap/zapcore"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"

	virtv1alpha1 "github.com/deckhouse/virtualization-controller/apis/v1alpha1"
	"github.com/deckhouse/virtualization-controller/pkg/controller"
)

var (
	log                  = logf.Log.WithName("cmd")
	resourcesSchemeFuncs = []func(*apiruntime.Scheme) error{
		clientgoscheme.AddToScheme,
		extv1.AddToScheme,
		virtv1alpha1.AddToScheme,
	}
)

func printVersion() {
	log.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	log.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
}

func main() {
	flag.Parse()

	defVerbose := fmt.Sprintf("%d", 1) // note flag values are strings
	verbose := defVerbose
	// visit actual flags passed in and if passed check -v and set verbose
	if verboseEnvVarVal := os.Getenv("VERBOSITY"); verboseEnvVarVal != "" {
		verbose = verboseEnvVarVal
	}
	if verbose == defVerbose {
		log.V(1).Info(fmt.Sprintf("Note: increase the -v level in the controller deployment for more detailed logging, eg. -v=%d or -v=%d\n", 2, 3))
	}
	verbosityLevel, err := strconv.Atoi(verbose)
	debug := false
	if err == nil && verbosityLevel > 1 {
		debug = true
	}

	// The logger instantiated here can be changed to any logger
	// implementing the logr.Logger interface. This logger will
	// be propagated through the whole operator, generating
	// uniform and structured logs.
	logf.SetLogger(zap.New(zap.Level(zapcore.Level(-1*verbosityLevel)), zap.UseDevMode(debug)))

	printVersion()

	//namespace := util.GetNamespace()

	// Get a config to talk to the apiserver
	cfg, err := config.GetConfig()
	if err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	leaderElectionNS := os.Getenv("OPERATOR_NAMESPACE")
	if leaderElectionNS == "" {
		leaderElectionNS = "default"
	}

	// Setup scheme for all resources
	scheme := apiruntime.NewScheme()
	for _, f := range resourcesSchemeFuncs {
		err := f(scheme)
		if err != nil {
			log.Error(err, "Failed to add to scheme")
			os.Exit(1)
		}
	}

	managerOpts := manager.Options{
		//Namespace:                  namespace,
		LeaderElection:             true,
		LeaderElectionNamespace:    leaderElectionNS,
		LeaderElectionID:           "d8-virt-operator-leader-election-helper",
		LeaderElectionResourceLock: "leases",
		Scheme:                     scheme,
	}

	// Create a new Manager to provide shared dependencies and start components
	mgr, err := manager.New(cfg, managerOpts)
	if err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	log.Info("Registering Components.")

	if err := extv1.AddToScheme(mgr.GetScheme()); err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	// Setup builders
	ctx := signals.SetupSignalHandler()

	if _, err := controller.NewCVMIController(ctx, mgr, log); err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	//if _, err := controller.NewVMIController(ctx, mgr, log); err != nil {
	//	log.Error(err, "")
	//	os.Exit(1)
	//}
	//
	//if _, err := controller.NewVMDController(ctx, mgr, log); err != nil {
	//	log.Error(err, "")
	//	os.Exit(1)
	//}

	log.Info("Starting the Manager.")

	// Start the Manager
	if err := mgr.Start(ctx); err != nil {
		log.Error(err, "manager exited non-zero")
		os.Exit(1)
	}
}
