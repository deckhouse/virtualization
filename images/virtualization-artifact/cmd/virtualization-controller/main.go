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
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	appconfig "github.com/deckhouse/virtualization-controller/pkg/config"
	"github.com/deckhouse/virtualization-controller/pkg/controller/cvi"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vd"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vdsnapshot"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vi"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmbda"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmclass"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmip"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmiplease"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmsnapshot"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/client/kubeclient"
	virtv2alpha1 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	pprofBindAddrEnv     = "PPROF_BIND_ADDRESS"
	logLevelEnv          = "LOG_LEVEL"
	logDebugVerbosityEnv = "LOG_DEBUG_VERBOSITY"
	logFormatEnv         = "LOG_FORMAT"
	logOutputEnv         = "LOG_OUTPUT"
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

	var logDebugVerbosity int
	flag.IntVar(&logDebugVerbosity, "log-debug-verbosity", int(defaultDebugVerbosity), "log debug verbosity")

	var logFormat string
	flag.StringVar(&logFormat, "log-format", os.Getenv(logFormatEnv), "log format")

	var logOutput string
	flag.StringVar(&logOutput, "log-output", os.Getenv(logOutputEnv), "log output")

	var pprofBindAddr string
	flag.StringVar(&pprofBindAddr, "pprof-bind-address", os.Getenv(pprofBindAddrEnv), "enable pprof")

	flag.Parse()

	log := logger.New(logger.Options{
		Level:          logLevel,
		DebugVerbosity: logDebugVerbosity,
		Format:         logFormat,
		Output:         logOutput,
	})

	logger.SetDefaultLogger(log)

	printVersion(log)

	controllerNamespace, err := appconfig.GetRequiredEnvVar(common.PodNamespaceVar)
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

	// Get a config to talk to the apiserver
	cfg, err := config.GetConfig()
	if err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}

	// Override content type to JSON so proxy can rewrite payloads.
	cfg.ContentType = apiruntime.ContentTypeJSON
	cfg.NegotiatedSerializer = clientgoscheme.Codecs.WithoutConversion()

	leaderElectionNS := os.Getenv(common.PodNamespaceVar)
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
	}
	if pprofBindAddr != "" {
		managerOpts.PprofBindAddress = pprofBindAddr
	}

	vmCIDRsRaw := os.Getenv(common.VirtualMachineCIDRs)
	if vmCIDRsRaw == "" {
		log.Error("Failed to get virtualMachineCIDRs: virtualMachineCIDRs not found, but required")
		os.Exit(1)
	}
	virtualMachineCIDRs := strings.Split(vmCIDRsRaw, ",")

	virtualMachineIPLeasesRetentionDuration := os.Getenv(common.VirtualMachineIPLeasesRetentionDuration)
	if virtualMachineIPLeasesRetentionDuration == "" {
		log.Info("virtualMachineIPLeasesRetentionDuration not found -> set default value '10m'")
		virtualMachineIPLeasesRetentionDuration = "10m"
	}

	storageClassForVirtualImageOnPVC := os.Getenv(common.VirtualImageStorageClass)
	if storageClassForVirtualImageOnPVC == "" {
		log.Info("virtualImages.storageClassName not found in ModuleConfig, default storage class will be used for images on PVCs.")
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

	if err = indexer.IndexALL(ctx, mgr); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}

	if _, err = cvi.NewController(ctx, mgr, log, importSettings.ImporterImage, importSettings.UploaderImage, importSettings.Requirements, dvcrSettings, controllerNamespace); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}

	if _, err = vd.NewController(ctx, mgr, log, importSettings.ImporterImage, importSettings.UploaderImage, importSettings.Requirements, dvcrSettings); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}

	if _, err = vi.NewController(ctx, mgr, log, importSettings.ImporterImage, importSettings.UploaderImage, importSettings.Requirements, dvcrSettings, storageClassForVirtualImageOnPVC); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}

	if err = vm.SetupController(ctx, mgr, log, dvcrSettings); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}
	if err = vm.SetupGC(mgr, log, gcSettings.VMIMigration); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}

	if _, err = vmbda.NewController(ctx, mgr, log, controllerNamespace); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}

	if _, err = vmip.NewController(ctx, mgr, log, virtualMachineCIDRs); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}

	if _, err = vmiplease.NewController(ctx, mgr, log, virtualMachineIPLeasesRetentionDuration); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}

	if _, err = vmclass.NewController(ctx, mgr, log); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}

	if _, err = vdsnapshot.NewController(ctx, mgr, log, virtClient); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}

	if err = vmsnapshot.NewController(ctx, mgr, log, virtClient); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}

	if err = vmop.SetupController(ctx, mgr, virtClient, log); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}
	if err = vmop.SetupGC(mgr, log, gcSettings.VMOP); err != nil {
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

func printVersion(log *slog.Logger) {
	log.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	log.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
}
