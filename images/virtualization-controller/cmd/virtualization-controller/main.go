package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"

	"go.uber.org/zap/zapcore"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	apiruntimeschema "k8s.io/apimachinery/pkg/runtime/schema"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	virtv1 "kubevirt.io/api/core/v1"
	cdiv1beta1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"

	virtv2alpha1 "github.com/deckhouse/virtualization-controller/api/v1alpha2"
	"github.com/deckhouse/virtualization-controller/pkg/common"
	appconfig "github.com/deckhouse/virtualization-controller/pkg/config"
	"github.com/deckhouse/virtualization-controller/pkg/controller"
	"github.com/deckhouse/virtualization-controller/pkg/controller/ipam"
)

var (
	log                  = logf.Log.WithName("cmd")
	resourcesSchemeFuncs = []func(*apiruntime.Scheme) error{
		clientgoscheme.AddToScheme,
		extv1.AddToScheme,
		virtv2alpha1.AddToScheme,
		cdiv1beta1.AddToScheme,
		virtv1.AddToScheme,
	}
	importerImage       string
	uploaderImage       string
	controllerNamespace string
)

const (
	defaultVerbosity      = "1"
	kubevirtCoreGroupName = "x.virtualization.deckhouse.io"
	cdiCoreGroupName      = "x.virtualization.deckhouse.io"
)

func init() {
	importerImage = getRequiredEnvVar(common.ImporterPodImageNameVar)
	uploaderImage = getRequiredEnvVar(common.UploaderPodImageNameVar)
	controllerNamespace = getRequiredEnvVar(common.PodNamespaceVar)

	overrideKubevirtCoreGroupName(kubevirtCoreGroupName)
	overrideCDICoreGroupName(cdiCoreGroupName)
}

func setupLogger() {
	verbose := defaultVerbosity
	if verboseEnvVarVal := os.Getenv("VERBOSITY"); verboseEnvVarVal != "" {
		verbose = verboseEnvVarVal
	}
	// visit actual flags passed in and if passed check -v and set verbose
	if fv := flag.Lookup("v"); fv != nil {
		verbose = fv.Value.String()
	}
	if verbose == defaultVerbosity {
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
}

func printVersion() {
	log.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	log.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
}

func getRequiredEnvVar(name string) string {
	val := os.Getenv(name)
	if val == "" {
		log.Error(fmt.Errorf("environment variable %q undefined", name), "")
	}
	return val
}

func overrideKubevirtCoreGroupName(groupName string) {
	virtv1.GroupVersion.Group = groupName
	virtv1.SchemeGroupVersion.Group = groupName
	virtv1.StorageGroupVersion.Group = groupName
	for i := range virtv1.GroupVersions {
		virtv1.GroupVersions[i].Group = groupName
	}

	virtv1.VirtualMachineInstanceGroupVersionKind.Group = groupName
	virtv1.VirtualMachineInstanceReplicaSetGroupVersionKind.Group = groupName
	virtv1.VirtualMachineInstancePresetGroupVersionKind.Group = groupName
	virtv1.VirtualMachineGroupVersionKind.Group = groupName
	virtv1.VirtualMachineInstanceMigrationGroupVersionKind.Group = groupName
	virtv1.KubeVirtGroupVersionKind.Group = groupName

	virtv1.SchemeBuilder = apiruntime.NewSchemeBuilder(virtv1.AddKnownTypesGenerator([]apiruntimeschema.GroupVersion{virtv1.GroupVersion}))
	virtv1.AddToScheme = virtv1.SchemeBuilder.AddToScheme
}

func overrideCDICoreGroupName(groupName string) {
	cdiv1beta1.SchemeGroupVersion.Group = groupName
	cdiv1beta1.CDIGroupVersionKind.Group = groupName

	cdiv1beta1.SchemeBuilder = apiruntime.NewSchemeBuilder(addKnownTypes)
	cdiv1beta1.AddToScheme = cdiv1beta1.SchemeBuilder.AddToScheme
}

// Adds the list of known types to Scheme.
func addKnownTypes(scheme *apiruntime.Scheme) error {
	scheme.AddKnownTypes(cdiv1beta1.SchemeGroupVersion,
		&cdiv1beta1.DataVolume{},
		&cdiv1beta1.DataVolumeList{},
		&cdiv1beta1.CDIConfig{},
		&cdiv1beta1.CDIConfigList{},
		&cdiv1beta1.CDI{},
		&cdiv1beta1.CDIList{},
		&cdiv1beta1.StorageProfile{},
		&cdiv1beta1.StorageProfileList{},
		&cdiv1beta1.DataSource{},
		&cdiv1beta1.DataSourceList{},
		&cdiv1beta1.DataImportCron{},
		&cdiv1beta1.DataImportCronList{},
		&cdiv1beta1.ObjectTransfer{},
		&cdiv1beta1.ObjectTransferList{},
		&cdiv1beta1.VolumeImportSource{},
		&cdiv1beta1.VolumeImportSourceList{},
		&cdiv1beta1.VolumeUploadSource{},
		&cdiv1beta1.VolumeUploadSourceList{},
		&cdiv1beta1.VolumeCloneSource{},
		&cdiv1beta1.VolumeCloneSourceList{},
	)
	metav1.AddToGroupVersion(scheme, cdiv1beta1.SchemeGroupVersion)
	return nil
}

func main() {
	flag.Parse()

	setupLogger()
	printVersion()

	dvcrSettings, err := appconfig.LoadDVCRSettingsFromEnvs(controllerNamespace)
	if err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	// Get a config to talk to the apiserver
	cfg, err := config.GetConfig()
	if err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	leaderElectionNS := os.Getenv(common.PodNamespaceVar)
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
		// This controller watches resources in all namespaces.
		LeaderElection:             true,
		LeaderElectionNamespace:    leaderElectionNS,
		LeaderElectionID:           "d8-virt-operator-leader-election-helper",
		LeaderElectionResourceLock: "leases",
		Scheme:                     scheme,
	}

	vmCIDRsRaw := os.Getenv(common.VirtualMachineCIDRs)
	if vmCIDRsRaw == "" {
		log.Error(errors.New("virtualMachineCIDRs not found, but required"), "Failed to get virtualMachineCIDRs")
		os.Exit(1)
	}
	virtualMachineCIDRs := strings.Split(vmCIDRsRaw, ",")

	// Create a new Manager to provide shared dependencies and start components
	mgr, err := manager.New(cfg, managerOpts)
	if err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	log.Info("Registering Components.")

	// Setup context to gracefully handle termination.
	ctx := signals.SetupSignalHandler()

	if _, err := controller.NewVMDController(ctx, mgr, log, importerImage, uploaderImage, dvcrSettings); err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	if _, err := controller.NewCVMIController(ctx, mgr, log, importerImage, uploaderImage, controllerNamespace, dvcrSettings); err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	if _, err := controller.NewVMIController(ctx, mgr, log, importerImage, uploaderImage, dvcrSettings); err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	if _, err := controller.NewVMController(ctx, mgr, log, dvcrSettings); err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	if _, err := controller.NewVMBDAController(ctx, mgr, log, controllerNamespace); err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	if _, err := ipam.NewClaimController(ctx, mgr, log, virtualMachineCIDRs); err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	if _, err := ipam.NewLeaseController(ctx, mgr, log); err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	log.Info("Starting the Manager.")

	// Start the Manager
	if err := mgr.Start(ctx); err != nil {
		log.Error(err, "manager exited non-zero")
		os.Exit(1)
	}
}
