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

package app

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/spf13/cobra"
	"go.uber.org/zap/zapcore"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"

	"vm-route-forge/cmd/vm-route-forge/app/options"
	vmipcache "vm-route-forge/internal/cache"
	"vm-route-forge/internal/controller/route"
	"vm-route-forge/internal/informer"
	"vm-route-forge/internal/netlinkmanager"
	"vm-route-forge/internal/netlinkwrap"
	"vm-route-forge/internal/runnablegroup"
	"vm-route-forge/internal/server"
)

const long = `
                                       _              __
__   ___ __ ___        _ __ ___  _   _| |_ ___       / _| ___  _ __ __ _  ___
\ \ / / '_ ` + "`" + ` _ \ _____| '__/ _ \| | | | __/ _ \_____| |_ / _ \| '__/ _` + "`" + ` |/ _ \
 \ V /| | | | | |_____| | | (_) | |_| | ||  __/_____|  _| (_) | | | (_| |  __/
  \_/ |_| |_| |_|     |_|  \___/ \__,_|\__\___|     |_|  \___/|_|  \__, |\___|
                                                                   |___/
Managing virtual machine routes
`

const (
	appName = "vm-route-forge"
	// The count of workers that will be started for the route controller.
	// We are currently supporting only one worker.
	countWorkersRouteController = 1
)

var (
	log = ctrl.Log.WithName(appName)
)

func NewVmRouteForgeCommand() *cobra.Command {
	opts := options.NewOptions()
	cmd := &cobra.Command{
		Short: "Managing virtual machine routes",
		Long:  long,
		RunE: func(c *cobra.Command, args []string) error {
			return run(opts)
		},
	}
	opts.Flags(cmd.Flags())
	return cmd
}

func setupLogger(verbosity int) {
	debug := false
	if verbosity > 1 {
		debug = true
	}

	// The logger instantiated here can be changed to any logger
	// implementing the logr.Logger interface. This logger will
	// be propagated through the whole operator, generating
	// uniform and structured logs.
	logf.SetLogger(zap.New(zap.Level(zapcore.Level(-1*verbosity)), zap.UseDevMode(debug)))
}

func run(opts options.Options) error {
	setupLogger(opts.Verbosity)
	var parsedCIDRs []*net.IPNet
	for _, cidr := range opts.Cidrs {
		_, parsedCIDR, err := net.ParseCIDR(cidr)
		if err != nil || parsedCIDR == nil {
			log.Error(err, "failed to parse passed CIDRs")
			return err
		}
		parsedCIDRs = append(parsedCIDRs, parsedCIDR)
	}
	log.Info(fmt.Sprintf("Got CIDRs to manage: %+v", opts.Cidrs))

	if opts.DryRun {
		log.Info("Dry run mode is enabled, will not change network rules and routes")
	}

	routeTableID := netlinkmanager.DefaultCiliumRouteTable
	routeTableIDStr := opts.RouteTableID
	if routeTableIDStr != "" {
		tableId, err := strconv.ParseInt(routeTableIDStr, 10, 32)
		if err != nil {
			log.Error(err, "failed to parse Cilium table id, should be integer")
			return err
		}
		routeTableID = int(tableId)
	}
	log.Info(fmt.Sprintf("Use route table id %d", routeTableID))

	// Load configuration to connect to Kubernetes API Server.
	kubeCfg, err := config.GetConfig()
	if err != nil {
		log.Error(err, "Failed to load Kubernetes config")
		return err
	}
	kubeClient, err := kubernetes.NewForConfig(kubeCfg)
	if err != nil {
		log.Error(err, "Failed to create Kubernetes client")
		return err
	}

	ctx := signals.SetupSignalHandler()

	vmSharedInformerFactory, err := informer.VirtualizationInformerFactory(kubeCfg)
	if err != nil {
		log.Error(err, "Failed to create virtualization shared factory")
		return err
	}
	go vmSharedInformerFactory.Virtualization().V1alpha2().VirtualMachines().Informer().Run(ctx.Done())

	ciliumSharedInformerFactory, err := informer.CiliumInformerFactory(kubeCfg)
	if err != nil {
		log.Error(err, "Failed to create cilium shared factory")
		return err
	}
	go ciliumSharedInformerFactory.Cilium().V2().CiliumNodes().Informer().Run(ctx.Done())

	sharedCache := vmipcache.NewCache()

	nlWrapper := netlinkwrap.NewFuncs()
	if opts.DryRun {
		nlWrapper = netlinkwrap.DryRunFuncs()
	}

	netMgr := netlinkmanager.New(sharedCache,
		log,
		routeTableID,
		parsedCIDRs,
		nlWrapper,
	)

	routeWatcher, err := route.WatchFactory(
		ctx,
		route.KindRouteWatcher(opts.KindRouteWatcher),
		parsedCIDRs,
		sharedCache,
		routeTableID,
		nlWrapper,
		log,
	)
	if err != nil {
		log.Error(err, "Failed to create route watcher")
		return err
	}
	defer routeWatcher.Stop()

	routeCtrl, err := route.NewController(
		vmSharedInformerFactory.Virtualization().V1alpha2().VirtualMachines(),
		ciliumSharedInformerFactory.Cilium().V2().CiliumNodes(),
		routeWatcher,
		netMgr,
		log,
	)
	if err != nil {
		log.Error(err, "Failed to create route controller")
		return err
	}

	serverOptions := server.Options{
		HealthProbeBindAddress: opts.ProbeAddr,
		PprofBindAddress:       opts.PprofAddr,
	}
	srv, err := server.NewServer(
		kubeClient,
		serverOptions,
		log,
	)
	if err != nil {
		log.Error(err, "Failed to create server")
		return err
	}
	runnableGroup := runnablegroup.NewRunnableGroup()
	runnableGroup.Add(srv)
	runnableGroup.Add(&wrapRouteController{routeCtrl: routeCtrl, count: countWorkersRouteController})

	return runnableGroup.Run(ctx)
}

type wrapRouteController struct {
	routeCtrl *route.Controller
	count     int
}

func (c *wrapRouteController) Run(ctx context.Context) error {
	return c.routeCtrl.Run(ctx, c.count)
}
