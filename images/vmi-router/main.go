/*
Copyright 2023,2024 Flant JSC

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
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"

	ciliumv2 "github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2"
	virtv1alpha2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"vmi-router/controllers"
	"vmi-router/netlinkmanager"
	"vmi-router/netutil"
)

const (
	defaultVerbosity      = "1"
	appName               = "vmi-router"
	NodeNameEnv           = "NODE_NAME"
	CiliumRouteTableIdEnv = "CILIUM_ROUTE_TABLE_ID"
)

var (
	log                  = ctrl.Log.WithName(appName)
	nodeName             = os.Getenv(NodeNameEnv)
	resourcesSchemeFuncs = []func(*runtime.Scheme) error{
		clientgoscheme.AddToScheme,
		ciliumv2.AddToScheme,
		virtv1alpha2.AddToScheme,
	}
)

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

func main() {
	var cidrs netutil.CIDRSet
	var dryRun bool
	var metricsAddr string
	var probeAddr string
	flag.Var(&cidrs, "cidr", "CIDRs enabled to route (multiple flags allowed)")
	flag.BoolVar(&dryRun, "dry-run", false, "Don't perform any changes on the node.")
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":0", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":0", "The address the probe endpoint binds to.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	setupLogger()

	var parsedCIDRs []*net.IPNet
	for _, cidr := range cidrs {
		_, parsedCIDR, err := net.ParseCIDR(cidr)
		if err != nil || parsedCIDR == nil {
			log.Error(err, "failed to parse passed CIDRs")
			os.Exit(1)
		}
		parsedCIDRs = append(parsedCIDRs, parsedCIDR)
	}
	log.Info(fmt.Sprintf("Got CIDRs to manage: %+v", cidrs))

	if dryRun {
		log.Info("Dry run mode is enabled, will not change network rules and routes")
	}

	ciliumTableId := netlinkmanager.DefaultCiliumRouteTable
	ciliumTableIdStr := os.Getenv(CiliumRouteTableIdEnv)
	if ciliumTableIdStr != "" {
		tableId, err := strconv.ParseInt(ciliumTableIdStr, 10, 32)
		if err != nil {
			log.Error(err, "failed to parse Cilium table id, should be integer")
			os.Exit(1)
		}
		ciliumTableId = int(tableId)
	}
	log.Info(fmt.Sprintf("Use cilium route table id %d", ciliumTableId))

	// Load configuration to connect to Kubernetes API Server.
	kubeCfg, err := config.GetConfig()
	if err != nil {
		log.Error(err, "Failed to load Kubernetes config")
		os.Exit(1)
	}

	// Setup scheme for all used resources (needed for controller-runtime).
	scheme := runtime.NewScheme()
	for _, f := range resourcesSchemeFuncs {
		err = f(scheme)
		if err != nil {
			log.Error(err, "Failed to add to scheme")
			os.Exit(1)
		}
	}

	// This controller watches resources in all namespaces without leader election.
	// Start metrics and health probe listeners on random ports as hostNetwork is used.
	managerOpts := manager.Options{
		LeaderElection: false,
		Scheme:         scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
	}

	mgr, err := ctrl.NewManager(kubeCfg, managerOpts)
	if err != nil {
		log.Error(err, "Unable to create manager")
		os.Exit(1)
	}

	// Setup context to gracefully handle termination.
	ctx := signals.SetupSignalHandler()

	// Create netlink manager.
	netlinkMgr := netlinkmanager.New(mgr.GetClient(), log, ciliumTableId, parsedCIDRs, dryRun)

	// Setup main controller with its dependencies.
	if err = controllers.NewVMRouterController(mgr, log, netlinkMgr); err != nil {
		log.Error(err, "Unable to add vmi router controller to manager")
		os.Exit(1)
	}

	// Init rules and cleanup unused routes at start.
	err = mgr.Add(manager.RunnableFunc(func(ctx context.Context) error {
		log.Info("Synchronize route rules at start")
		err := netlinkMgr.SyncRules()
		if err != nil {
			log.Error(err, fmt.Sprintf("failed to synchronize routing rules ar start"))
			return err
		}

		log.Info("Synchronize VM routes at start")
		err = netlinkMgr.SyncRoutes(ctx)
		if err != nil {
			log.Error(err, fmt.Sprintf("failed to synchronize VM routes at start"))
			return err
		}

		return nil
	}))
	if err != nil {
		log.Error(err, "Add routes synchronizer")
		os.Exit(1)
	}

	if err = mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		log.Error(err, "Unable to set up health check")
		os.Exit(1)
	}
	if err = mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		log.Error(err, "Unable to set up ready check")
		os.Exit(1)
	}

	log.Info("Starting manager")
	if err = mgr.Start(ctx); err != nil {
		log.Error(err, "Unable to start manager")
		os.Exit(1)
	}
}
