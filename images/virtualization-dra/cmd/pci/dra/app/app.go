/*
Copyright 2026 Flant JSC

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
	"time"

	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/component-base/cli/flag"

	"github.com/deckhouse/virtualization-dra/internal/cdi"
	"github.com/deckhouse/virtualization-dra/internal/pci"
	"github.com/deckhouse/virtualization-dra/internal/plugin"
	"github.com/deckhouse/virtualization-dra/pkg/cli"
	"github.com/deckhouse/virtualization-dra/pkg/logger"
)

func NewVirtualizationDraPCICommand() *cobra.Command {
	o := &draOptions{
		logging: &logger.Options{},
	}

	cmd := &cobra.Command{
		Use:           "virtualization-dra-pci",
		Short:         "virtualization-dra-pci",
		SilenceUsage:  true,
		SilenceErrors: true,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			o.Complete()
			return o.Validate()
		},
		RunE: o.Run,
	}

	fs := cmd.Flags()
	for _, f := range o.NamedFlags().FlagSets {
		fs.AddFlagSet(f)
	}

	return cmd
}

type draOptions struct {
	DriverName     string
	Kubeconfig     string
	NodeName       string
	CDIRoot        string
	HealthzPort    int
	RescanInterval time.Duration

	logging *logger.Options
}

func (o *draOptions) Complete() {
	log := o.logging.Complete()
	logger.SetDefaultLogger(log)
}

func (o *draOptions) NamedFlags() (fs flag.NamedFlagSets) {
	mfs := fs.FlagSet("virtualization-pci plugin")
	mfs.StringVar(&o.DriverName, "driver-name", pci.DriverName, "Driver name")
	mfs.StringVar(&o.Kubeconfig, "kubeconfig", cli.GetStringEnv("KUBECONFIG", ""), "Path to kubeconfig file")
	mfs.StringVar(&o.NodeName, "node-name", cli.GetStringEnv("NODE_NAME", ""), "Node name")
	mfs.StringVar(&o.CDIRoot, "cdi-root", cli.GetStringEnv("CDI_ROOT", cdi.SpecDir), "CDI root")
	mfs.IntVar(&o.HealthzPort, "healthz-port", cli.GetIntEnv("HEALTHZ_PORT", 51516), "Healthz port")
	mfs.DurationVar(&o.RescanInterval, "rescan-interval", 5*time.Minute, "PCI bus rescan interval")

	o.logging.AddFlags(fs.FlagSet("logging"))
	plugin.AddFlags(fs.FlagSet("plugin"))

	return fs
}

func (o *draOptions) Validate() error {
	if o.NodeName == "" {
		return fmt.Errorf("nodeName is required")
	}
	if o.CDIRoot == "" {
		return fmt.Errorf("cdiRoot is required")
	}

	return nil
}

func (o *draOptions) Run(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()

	cfg, err := clientcmd.BuildConfigFromFlags("", o.Kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to get rest config: %w", err)
	}

	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	pciCDIManager, err := cdi.NewManager(o.CDIRoot, "pci", o.DriverName, o.NodeName, "DRA_PCI")
	if err != nil {
		return fmt.Errorf("failed to create CDI manager: %w", err)
	}

	pciStore, err := pci.NewAllocationStore(ctx, o.NodeName, pciCDIManager, o.RescanInterval)
	if err != nil {
		return fmt.Errorf("failed to create PCI store: %w", err)
	}

	mgr, err := plugin.NewManager(o.DriverName, o.NodeName, client, pciStore, o.HealthzPort, false)
	if err != nil {
		return fmt.Errorf("failed to create manager: %w", err)
	}

	return mgr.Run(ctx)
}
