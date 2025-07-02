/*
Copyright 2025 Flant JSC

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
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	deckhouselog "github.com/deckhouse/deckhouse/pkg/log"
	virtualizationconfig "github.com/deckhouse/virtualization-controller/pkg/config"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	mc "github.com/deckhouse/virtualization-controller/pkg/controller/moduleconfig"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization-controller/pkg/migration"
	"github.com/deckhouse/virtualization-controller/pkg/version"
	"github.com/deckhouse/virtualization/api/client/kubeclient"
)

func NewVirtualizationControllerCommand() *cobra.Command {
	opts := &options{}
	cmd := &cobra.Command{
		Use:           "virtualization-controller",
		Short:         "virtualization-controller",
		Long:          "virtualization-controller",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE:          opts.Run,
	}

	opts.AddFlags(cmd.Flags())
	return cmd
}

type options struct {
	Config                 string
	PprofBindAddr          string
	MetricsBindAddr        string
	LogLevel               string
	LogOutput              string
	LogFormat              string
	LogDebugControllerList []string
	LogDebugVerbosity      int
	LeaderElection         bool
}

func (o *options) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.Config, "config", "/etc/virtualization-controller/config.yaml", "path to config file")
	fs.StringVar(&o.PprofBindAddr, "pprof-bind-address", "", "enable pprof")
	fs.StringVar(&o.MetricsBindAddr, "metrics-bind-address", "8080", "metric bind address")
	fs.StringVar(&o.LogLevel, "log-level", "info", "log level")
	fs.StringVar(&o.LogOutput, "log-output", "", "log output")
	fs.StringVar(&o.LogFormat, "log-format", "json", "log format")
	fs.StringSliceVar(&o.LogDebugControllerList, "log-debug-controller-list", nil, "log debug controller list")
	fs.IntVar(&o.LogDebugVerbosity, "log-debug-verbosity", 0, "log debug verbosity")
	fs.BoolVar(&o.LeaderElection, "leader-election", true, "enable leader election")
}

func (o *options) Run(cmd *cobra.Command, _ []string) error {
	log := logger.NewLogger(o.LogLevel, o.LogOutput, o.LogDebugVerbosity)
	logger.SetDefaultLogger(log)

	printVersion(log)

	configuration, err := virtualizationconfig.Load(o.Config)
	if err != nil {
		return err
	}

	cfg, err := config.GetConfig()
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}

	// Override content type to JSON so proxy can rewrite payloads.
	cfg.ContentType = apiruntime.ContentTypeJSON
	cfg.NegotiatedSerializer = clientgoscheme.Codecs.WithoutConversion()

	scheme, err := newScheme()
	if err != nil {
		return err
	}

	managerOpts := manager.Options{
		LeaderElection:             o.LeaderElection,
		LeaderElectionNamespace:    configuration.Spec.Namespace,
		LeaderElectionID:           "d8-virtualization-controller",
		LeaderElectionResourceLock: "leases",
		Scheme:                     scheme,
		Metrics: metricsserver.Options{
			BindAddress: o.MetricsBindAddr,
		},
		PprofBindAddress: o.PprofBindAddr,
	}

	mgr, err := manager.New(cfg, managerOpts)
	if err != nil {
		return fmt.Errorf("failed to create manager: %w", err)
	}

	virtualizationClient, err := kubeclient.GetClientFromRESTConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to create virtualization kubeclient: %w", err)
	}

	log.Info("Registering Components.")
	onlyMigrationClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return fmt.Errorf("failed to create onlyMigrationClient: %w", err)
	}

	mCtrl, err := migration.NewController(onlyMigrationClient, log)
	if err != nil {
		return fmt.Errorf("failed to create migration controller: %w", err)
	}
	ctx := cmd.Context()
	mCtrl.Run(ctx)

	if err = indexer.IndexALL(ctx, mgr); err != nil {
		return fmt.Errorf("failed to index all resources: %w", err)
	}

	for controllerName, setupController := range controllers {
		loggerForController := logger.NewControllerLogger(controllerName, o.LogLevel, o.LogOutput, o.LogDebugVerbosity, o.LogDebugControllerList)
		err = setupController(ctx, mgr, loggerForController, configuration.DeepCopy(), virtualizationClient)
		if err != nil {
			return fmt.Errorf("failed to setup %s controller: %w", controllerName, err)
		}
	}

	if err = mc.SetupWebhookWithManager(mgr); err != nil {
		return fmt.Errorf("failed to setup module config webhook: %w", err)
	}

	log.Info("Starting the Manager.")

	if err = mgr.Start(ctx); err != nil {
		return fmt.Errorf("manager exired non-zero: %w", err)
	}
	return nil
}

func printVersion(log *deckhouselog.Logger) {
	log.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	log.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
	log.Info(fmt.Sprintf("Edition: %s", version.GetEdition()))
}
