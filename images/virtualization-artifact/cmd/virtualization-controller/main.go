/*
Copyright 2024 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strconv"
	"strings"

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	virtv1 "kubevirt.io/api/core/v1"
	cdiv1beta1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/deckhouse/deckhouse/pkg/log"
	appconfig "github.com/deckhouse/virtualization-controller/pkg/config"
	"github.com/deckhouse/virtualization-controller/pkg/controller/cvi"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	mc "github.com/deckhouse/virtualization-controller/pkg/controller/moduleconfig"
	mcapi "github.com/deckhouse/virtualization-controller/pkg/controller/moduleconfig/api"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vd"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vdsnapshot"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vi"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmbda"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmclass"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmip"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmiplease"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmrestore"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmsnapshot"
	workloadupdater "github.com/deckhouse/virtualization-controller/pkg/controller/workload-updater"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization-controller/pkg/migration"
	"github.com/deckhouse/virtualization-controller/pkg/version"
	"github.com/deckhouse/virtualization/api/client/kubeclient"
	virtv2alpha1 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	logDebugControllerListEnv = "LOG_DEBUG_CONTROLLER_LIST"
	logDebugVerbosityEnv      = "LOG_DEBUG_VERBOSITY"
	logFormatEnv              = "LOG_FORMAT"
	logLevelEnv               = "LOG_LEVEL"
	logOutputEnv              = "LOG_OUTPUT"

	metricsBindAddrEnv                         = "METRICS_BIND_ADDRESS"
	podNamespaceEnv                            = "POD_NAMESPACE"
	pprofBindAddrEnv                           = "PPROF_BIND_ADDRESS"
	virtualMachineCIDRsEnv                     = "VIRTUAL_MACHINE_CIDRS"
	virtualMachineIPLeasesRetentionDurationEnv = "VIRTUAL_MACHINE_IP_LEASES_RETENTION_DURATION"

	FirmwareImageEnv = "FIRMWARE_IMAGE"
)

func main() {
	var logLevel string
	flag.StringVar(&logLevel, "log-level", os.Getenv(logLevelEnv), "log level")

	var err error
	var defaultDebugVerbosity int64
	logDebugVerbosityRaw := os.Getenv(logDebugVerbosityEnv)
	if logDebugVerbosityRaw != "" {
		defaultDebugVerbosity, err = strconv.ParseInt(logDebugVerbosityRaw, 10, 64)
		if err != nil {
			slog.Default().Error(err.Error())
			os.Exit(1)
		}
	}

	var logDebugControllerList []string
	fmt.Print(len(logDebugControllerList))
	logDebugControllerListRaw := os.Getenv(logDebugControllerListEnv)
	if logDebugControllerListRaw != "" {
		logDebugControllerListRaw = strings.ReplaceAll(logDebugControllerListRaw, " ", "")
		logDebugControllerList = strings.Split(logDebugControllerListRaw, ",")
	}

	var logDebugVerbosity int
	flag.IntVar(&logDebugVerbosity, "log-debug-verbosity", int(defaultDebugVerbosity), "log debug verbosity")

	var logOutput string
	flag.StringVar(&logOutput, "log-output", os.Getenv(logOutputEnv), "log output")

	var pprofBindAddr string
	flag.StringVar(&pprofBindAddr, "pprof-bind-address", os.Getenv(pprofBindAddrEnv), "enable pprof")

	var metricsBindAddr string
	flag.StringVar(&metricsBindAddr, "metrics-bind-address", getEnv(metricsBindAddrEnv, ":8080"), "metric bind address")

	var firmwareImage string
	flag.StringVar(&firmwareImage, "firmware-image", os.Getenv(FirmwareImageEnv), "Firmware image")

	flag.Parse()

	log := logger.NewLogger(logLevel, logOutput, logDebugVerbosity)
	logger.SetDefaultLogger(log)

	printVersion(log)

	if firmwareImage == "" {
		log.Error("firmware image is required")
		os.Exit(1)
	}

	controllerNamespace, err := appconfig.GetRequiredEnvVar(podNamespaceEnv)
	if err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}

	dvcrSettings, err := appconfig.LoadDVCRSettingsFromEnvs(controllerNamespace)
	if err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}

	importSettings, err := appconfig.LoadImportSettingsFromEnv()
	if err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}

	gcSettings, err := appconfig.LoadGcSettings()
	if err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}

	viStorageClassSettings := appconfig.LoadVirtualImageStorageClassSettings()
	vdStorageClassSettings := appconfig.LoadVirtualDiskStorageClassSettings()

	// Get a config to talk to the apiserver
	cfg, err := config.GetConfig()
	if err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}

	// Override content type to JSON so proxy can rewrite payloads.
	cfg.ContentType = apiruntime.ContentTypeJSON
	cfg.NegotiatedSerializer = clientgoscheme.Codecs.WithoutConversion()

	leaderElectionNS := os.Getenv(podNamespaceEnv)
	if leaderElectionNS == "" {
		leaderElectionNS = "default"
	}

	// Setup scheme for all resources
	scheme := apiruntime.NewScheme()

	for _, f := range []func(*apiruntime.Scheme) error{
		clientgoscheme.AddToScheme,
		extv1.AddToScheme,
		virtv2alpha1.AddToScheme,
		cdiv1beta1.AddToScheme,
		virtv1.AddToScheme,
		vsv1.AddToScheme,
		mcapi.AddToScheme,
	} {
		err = f(scheme)
		if err != nil {
			log.Error("Failed to add to scheme", logger.SlogErr(err))
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
		Metrics: metricsserver.Options{
			BindAddress: metricsBindAddr,
		},
	}
	if pprofBindAddr != "" {
		managerOpts.PprofBindAddress = pprofBindAddr
	}

	vmCIDRsRaw := os.Getenv(virtualMachineCIDRsEnv)
	if vmCIDRsRaw == "" {
		log.Error("Failed to get virtualMachineCIDRs: virtualMachineCIDRs not found, but required")
		os.Exit(1)
	}
	virtualMachineCIDRs := strings.Split(vmCIDRsRaw, ",")

	virtualMachineIPLeasesRetentionDuration := os.Getenv(virtualMachineIPLeasesRetentionDurationEnv)
	if virtualMachineIPLeasesRetentionDuration == "" {
		log.Info("virtualMachineIPLeasesRetentionDuration not found -> set default value '10m'")
		virtualMachineIPLeasesRetentionDuration = "10m"
	}

	// Create a new Manager to provide shared dependencies and start components
	mgr, err := manager.New(cfg, managerOpts)
	if err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}

	virtClient, err := kubeclient.GetClientFromRESTConfig(cfg)
	if err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}

	log.Info("Registering Components.")

	// Setup context to gracefully handle termination.
	ctx := signals.SetupSignalHandler()

	onlyMigrationClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}
	mCtrl, err := migration.NewController(onlyMigrationClient, log)
	if err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}
	mCtrl.Run(ctx)

	if err = indexer.IndexALL(ctx, mgr); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}

	cviLogger := logger.NewControllerLogger(cvi.ControllerName, logLevel, logOutput, logDebugVerbosity, logDebugControllerList)
	if _, err = cvi.NewController(ctx, mgr, cviLogger, importSettings.ImporterImage, importSettings.UploaderImage, importSettings.Requirements, dvcrSettings, controllerNamespace); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}

	vdLogger := logger.NewControllerLogger(vd.ControllerName, logLevel, logOutput, logDebugVerbosity, logDebugControllerList)
	if _, err = vd.NewController(ctx, mgr, vdLogger, importSettings.ImporterImage, importSettings.UploaderImage, importSettings.Requirements, dvcrSettings, vdStorageClassSettings); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}

	viLogger := logger.NewControllerLogger(vi.ControllerName, logLevel, logOutput, logDebugVerbosity, logDebugControllerList)
	if _, err = vi.NewController(ctx, mgr, viLogger, importSettings.ImporterImage, importSettings.UploaderImage, importSettings.BounderImage, importSettings.Requirements, dvcrSettings, viStorageClassSettings); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}

	vmLogger := logger.NewControllerLogger(vm.ControllerName, logLevel, logOutput, logDebugVerbosity, logDebugControllerList)
	if err = vm.SetupController(ctx, mgr, vmLogger, dvcrSettings, firmwareImage); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}
	if err = vm.SetupGC(mgr, vmLogger, gcSettings.VMIMigration); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}

	vmbdaLogger := logger.NewControllerLogger(vmbda.ControllerName, logLevel, logOutput, logDebugVerbosity, logDebugControllerList)
	if _, err = vmbda.NewController(ctx, mgr, virtClient, vmbdaLogger, controllerNamespace); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}

	vmipLogger := logger.NewControllerLogger(vmip.ControllerName, logLevel, logOutput, logDebugVerbosity, logDebugControllerList)
	if _, err = vmip.NewController(ctx, mgr, vmipLogger, virtualMachineCIDRs); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}

	vmipleaseLogger := logger.NewControllerLogger(vmiplease.ControllerName, logLevel, logOutput, logDebugVerbosity, logDebugControllerList)
	if _, err = vmiplease.NewController(ctx, mgr, vmipleaseLogger, virtualMachineIPLeasesRetentionDuration); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}

	vmclassLogger := logger.NewControllerLogger(vmclass.ControllerName, logLevel, logOutput, logDebugVerbosity, logDebugControllerList)
	if _, err = vmclass.NewController(ctx, mgr, controllerNamespace, vmclassLogger); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}

	vdsnapshotLogger := logger.NewControllerLogger(vdsnapshot.ControllerName, logLevel, logOutput, logDebugVerbosity, logDebugControllerList)
	if _, err = vdsnapshot.NewController(ctx, mgr, vdsnapshotLogger, virtClient); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}

	vmsnapshotLogger := logger.NewControllerLogger(vmsnapshot.ControllerName, logLevel, logOutput, logDebugVerbosity, logDebugControllerList)
	if err = vmsnapshot.NewController(ctx, mgr, vmsnapshotLogger, virtClient); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}

	vmrestoreLogger := logger.NewControllerLogger(vmrestore.ControllerName, logLevel, logOutput, logDebugVerbosity, logDebugControllerList)
	if err = vmrestore.NewController(ctx, mgr, vmrestoreLogger); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}

	vmopLogger := logger.NewControllerLogger(vmop.ControllerName, logLevel, logOutput, logDebugVerbosity, logDebugControllerList)
	if err = vmop.SetupController(ctx, mgr, vmopLogger); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}
	if err = vmop.SetupGC(mgr, vmopLogger, gcSettings.VMOP); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}

	if err = mc.SetupWebhookWithManager(mgr); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}

	if err = workloadupdater.SetupController(ctx, mgr, log, firmwareImage); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}

	log.Info("Starting the Manager.")

	// Start the Manager
	if err = mgr.Start(ctx); err != nil {
		log.Error("Manager exited non-zero", logger.SlogErr(err))
		os.Exit(1)
	}
}

func printVersion(log *log.Logger) {
	log.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	log.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
	log.Info(fmt.Sprintf("Edition: %s", version.GetEdition()))
}

func getEnv(env, defaultEnv string) string {
	if e, found := os.LookupEnv(env); found {
		return e
	}
	return defaultEnv
}
