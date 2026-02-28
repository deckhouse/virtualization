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
	"log/slog"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/component-base/cli/flag"

	"github.com/deckhouse/virtualization-dra/internal/cdi"
	"github.com/deckhouse/virtualization-dra/internal/featuregates"
	"github.com/deckhouse/virtualization-dra/internal/plugin"
	"github.com/deckhouse/virtualization-dra/internal/usb"
	usbgateway "github.com/deckhouse/virtualization-dra/internal/usb-gateway"
	"github.com/deckhouse/virtualization-dra/internal/usb-gateway/informer"
	"github.com/deckhouse/virtualization-dra/pkg/cli"
	"github.com/deckhouse/virtualization-dra/pkg/controller"
	"github.com/deckhouse/virtualization-dra/pkg/libusb"
	"github.com/deckhouse/virtualization-dra/pkg/logger"
	"github.com/deckhouse/virtualization-dra/pkg/usbip"
)

func NewVirtualizationDraUSBCommand() *cobra.Command {
	o := &draOptions{
		logging:      &logger.Options{},
		monitor:      libusb.NewDefaultMonitorConfig(),
		usbipdConfig: &usbip.USBIPDConfig{},
	}

	cmd := &cobra.Command{
		Use:           "virtualization-dra-usb",
		Short:         "virtualization-dra-usb",
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

	cmd.AddCommand(NewInitCommand())

	return cmd
}

type draOptions struct {
	DriverName  string
	Kubeconfig  string
	NodeName    string
	CDIRoot     string
	HealthzPort int

	logging      *logger.Options
	monitor      *libusb.MonitorConfig
	usbipdConfig *usbip.USBIPDConfig

	usbGatewayEnabled bool
}

func (o *draOptions) Complete() {
	log := o.logging.Complete()
	logger.SetDefaultLogger(log)

	o.usbGatewayEnabled = featuregates.Default().USBGatewayEnabled()
	if o.usbGatewayEnabled {
		if !o.usbipdConfig.ExportEnabled {
			slog.Warn("USB gateway is enabled but USBIPD export is disabled. Enabling USBIPD export.")
		}
		o.usbipdConfig.ExportEnabled = true
	}
}

func (o *draOptions) NamedFlags() (fs flag.NamedFlagSets) {
	mfs := fs.FlagSet("virtualization-usb plugin")
	mfs.StringVar(&o.DriverName, "driver-name", usb.DriverName, "Driver name")
	mfs.StringVar(&o.Kubeconfig, "kubeconfig", cli.GetStringEnv("KUBECONFIG", ""), "Path to kubeconfig file")
	mfs.StringVar(&o.NodeName, "node-name", cli.GetStringEnv("NODE_NAME", ""), "Node name")
	mfs.StringVar(&o.CDIRoot, "cdi-root", cli.GetStringEnv("CDI_ROOT", cdi.SpecDir), "CDI root")
	mfs.IntVar(&o.HealthzPort, "healthz-port", cli.GetIntEnv("HEALTHZ_PORT", 51515), "Healthz port")

	o.logging.AddFlags(fs.FlagSet("logging"))
	o.monitor.AddFlags(fs.FlagSet("usb-monitor"))
	o.usbipdConfig.AddFlags(fs.FlagSet("usbipd"))
	plugin.AddFlags(fs.FlagSet("plugin"))
	featuregates.AddFlags(fs.FlagSet("feature-gates"))

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

func (o *draOptions) Clients() (kubernetes.Interface, dynamic.Interface, error) {
	cfg, err := clientcmd.BuildConfigFromFlags("", o.Kubeconfig)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get rest config: %w", err)
	}

	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	dynamicClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	return client, dynamicClient, nil
}

func (o *draOptions) Run(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()

	client, dynamicClient, err := o.Clients()
	if err != nil {
		return err
	}

	monitor, err := o.monitor.Complete(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to create USB monitor: %w", err)
	}

	var usbGateway usbgateway.USBGateway

	group, ctx := errgroup.WithContext(ctx)

	if o.usbGatewayEnabled {
		usbipd, err := o.usbipdConfig.Complete(monitor)
		if err != nil {
			return fmt.Errorf("failed to create USBIPD: %w", err)
		}

		f := informer.NewFactory(client, nil)
		nodeInformer := f.Nodes()
		resourceSliceInformer := f.ResourceSlice()

		group.Go(func() error {
			return f.Run(ctx)
		})
		f.WaitForCacheSync(ctx.Done())

		usbGatewayController, err := usbgateway.NewUSBGatewayController(
			o.NodeName,
			o.usbipdConfig.Address,
			o.usbipdConfig.Port,
			client,
			dynamicClient,
			nodeInformer,
			resourceSliceInformer,
			usbip.New(),
		)
		if err != nil {
			return fmt.Errorf("failed to create USB gateway controller: %w", err)
		}

		group.Go(func() error {
			return usbipd.Run(ctx)
		})

		group.Go(func() error {
			return controller.Run(usbGatewayController, ctx, 1)
		})

		usbGateway = usbGatewayController
	}

	usbCDIManager, err := cdi.NewManager(o.CDIRoot, "usb", o.DriverName, o.NodeName, "DRA_USB")
	if err != nil {
		return fmt.Errorf("failed to create CDI manager: %w", err)
	}

	usbStore, err := usb.NewAllocationStore(ctx, o.NodeName, usbCDIManager, monitor, usbGateway, client)
	if err != nil {
		return fmt.Errorf("failed to create USB store: %w", err)
	}

	mgr, err := plugin.NewManager(o.DriverName, o.NodeName, client, usbStore, o.HealthzPort, o.usbGatewayEnabled)
	if err != nil {
		return fmt.Errorf("failed to create manager: %w", err)
	}

	group.Go(func() error {
		return mgr.Run(ctx)
	})

	return group.Wait()
}
