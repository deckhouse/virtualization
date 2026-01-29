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
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/component-base/cli/flag"

	"github.com/deckhouse/virtualization-dra/internal/featuregates"
	"github.com/deckhouse/virtualization-dra/internal/usb-gateway/controller/resourceclaim"
	"github.com/deckhouse/virtualization-dra/internal/usb-gateway/informer"
	"github.com/deckhouse/virtualization-dra/internal/usb-gateway/prepare"
	"github.com/deckhouse/virtualization-dra/internal/usbip"
	"github.com/deckhouse/virtualization-dra/pkg/controller"
	"github.com/deckhouse/virtualization-dra/pkg/libusb"
	"github.com/deckhouse/virtualization-dra/pkg/logger"
)

func NewUSBGatewayCommand() *cobra.Command {
	o := newUsbOptions()

	cmd := &cobra.Command{
		Use:           "usb-gateway",
		Short:         "USB gateway",
		Long:          "USB gateway",
		SilenceUsage:  true,
		SilenceErrors: true,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			o.Complete()
			return o.Validate()
		},
		RunE: o.Run,
	}

	cmd.AddCommand(NewInitCommand())

	fs := cmd.Flags()
	for _, f := range o.NamedFlags().FlagSets {
		fs.AddFlagSet(f)
	}

	return cmd
}

func newUsbOptions() *usbOptions {
	return &usbOptions{
		usbipdConfig: &usbip.USBIPDConfig{},
		logging:      &logger.Options{},
		monitor:      libusb.NewDefaultMonitorConfig(),
	}
}

type usbOptions struct {
	Kubeconfig                 string
	NodeName                   string
	Namespace                  string
	WireguardSystemNetworkName string
	PodIP                      string
	WireguardRouteTableID      int

	usbipdConfig *usbip.USBIPDConfig
	logging      *logger.Options
	monitor      *libusb.MonitorConfig

	wireguardEnabled bool
}

func (o *usbOptions) NamedFlags() (fs flag.NamedFlagSets) {
	mfs := fs.FlagSet("usb-gateway")
	mfs.StringVar(&o.Kubeconfig, "kubeconfig", o.Kubeconfig, "Path to kubeconfig file")
	mfs.StringVar(&o.NodeName, "node-name", os.Getenv("NODE_NAME"), "Node name")
	mfs.StringVar(&o.Namespace, "namespace", os.Getenv("NAMESPACE"), "Namespace")
	mfs.StringVar(&o.WireguardSystemNetworkName, "wireguard-system-network-name", "", "Wireguard system network name")
	mfs.StringVar(&o.PodIP, "pod-ip", os.Getenv("POD_IP"), "Pod IP")
	mfs.IntVar(&o.WireguardRouteTableID, "wireguard-route-table-id", o.WireguardRouteTableID, "Wireguard route table ID")

	o.usbipdConfig.AddFlags(fs.FlagSet("usbipd"))
	o.logging.AddFlags(fs.FlagSet("logging"))
	o.monitor.AddFlags(fs.FlagSet("usb-monitor"))
	featuregates.AddFlags(fs.FlagSet("feature-gates"))

	return fs
}

func (o *usbOptions) Complete() {
	log := o.logging.Complete()
	logger.SetDefaultLogger(log)

	o.wireguardEnabled = featuregates.Default().USBGatewayWireguardEnabled()
}

func (o *usbOptions) Validate() error {
	if o.NodeName == "" {
		return fmt.Errorf("NodeName is required")
	}
	if o.Namespace == "" {
		return fmt.Errorf("Namespace is required")
	}
	if o.wireguardEnabled {
		if o.WireguardSystemNetworkName == "" {
			return fmt.Errorf("WireguardSystemNetworkName is required if feature-gate USBGatewayWireguard is enabled")
		}
		if o.PodIP == "" {
			return fmt.Errorf("PodIP is required if feature-gate USBGatewayWireguard is enabled")
		}
	}

	return nil
}

func (o *usbOptions) USBIPD(ctx context.Context) (*usbip.USBIPD, error) {
	monitor, err := o.monitor.Complete(ctx, nil)
	if err != nil {
		return nil, err
	}

	usbipd, err := o.usbipdConfig.Complete(monitor)
	if err != nil {
		return nil, err
	}

	return usbipd, nil
}

func (o *usbOptions) Clients() (kubernetes.Interface, dynamic.Interface, error) {
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

func (o *usbOptions) Run(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()

	client, dynamicClient, err := o.Clients()
	if err != nil {
		return err
	}

	f := informer.NewFactory(client, nil)
	resourceClaimInformer := f.ResourceClaim()
	resourceSliceInformer := f.ResourceSlice()
	nodeInformer := f.Nodes()
	podInformer := f.Pods()

	f.Start(ctx.Done())
	f.WaitForCacheSync(ctx.Done())

	defer func() {
		if err := prepare.UnmarkNodeForUSBGateway(ctx, o.NodeName, dynamicClient); err != nil {
			slog.Error("failed to unmark node for USB gateway", slog.Any("error", err))
		}
	}()
	if err = prepare.MarkNodeForUSBGateway(ctx, o.NodeName, dynamicClient); err != nil {
		return fmt.Errorf("failed to mark node for USB gateway: %w", err)
	}

	usbipd, err := o.USBIPD(ctx)
	if err != nil {
		return err
	}

	usbIPInterface := usbip.New()
	resourceClaimController, err := resourceclaim.NewController(o.NodeName, o.usbipdConfig.Address, o.usbipdConfig.Port, client, resourceClaimInformer, resourceSliceInformer, nodeInformer, podInformer, usbIPInterface)
	if err != nil {
		return fmt.Errorf("failed to create resourceclaim controller: %w", err)
	}

	group, ctx := errgroup.WithContext(ctx)

	group.Go(func() error {
		return usbipd.Run(ctx)
	})

	group.Go(func() error {
		return controller.Run(resourceClaimController, ctx, 1)
	})

	return group.Wait()
}
