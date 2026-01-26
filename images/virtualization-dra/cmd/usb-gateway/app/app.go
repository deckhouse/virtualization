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
	"net"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/component-base/cli/flag"

	"github.com/deckhouse/virtualization-dra/internal/usb-gateway/controller/resourceclaim"
	"github.com/deckhouse/virtualization-dra/internal/usb-gateway/informer"
	"github.com/deckhouse/virtualization-dra/internal/usb-gateway/prepare"
	"github.com/deckhouse/virtualization-dra/internal/usbip"
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
			if err := o.Validate(); err != nil {
				return err
			}
			log := o.Logging.Complete()
			logger.SetDefaultLogger(log)
			return nil
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
		Logging: &logger.Options{},
		Monitor: libusb.NewDefaultMonitorConfig(),
	}
}

type usbOptions struct {
	Kubeconfig string
	NodeName   string
	PodIP      string
	USBIPPort  int
	Logging    *logger.Options
	Monitor    *libusb.MonitorConfig
}

func (o *usbOptions) NamedFlags() (fs flag.NamedFlagSets) {
	mfs := fs.FlagSet("usb-gateway")
	mfs.StringVar(&o.Kubeconfig, "kubeconfig", o.Kubeconfig, "Path to kubeconfig file")
	mfs.StringVar(&o.NodeName, "node-name", os.Getenv("NODE_NAME"), "Node name")
	mfs.StringVar(&o.PodIP, "pod-ip", os.Getenv("POD_IP"), "Pod IP")
	mfs.IntVar(&o.USBIPPort, "usbip-port", 3240, "USBIP port")

	o.Logging.AddFlags(fs.FlagSet("logging"))

	o.Monitor.AddFlags(fs.FlagSet("usb-monitor"))

	return fs
}

func (o *usbOptions) Validate() error {
	if o.NodeName == "" {
		return fmt.Errorf("NodeName is required")
	}
	if o.PodIP == "" {
		return fmt.Errorf("PodIP is required")
	}
	if net.ParseIP(o.PodIP) == nil {
		return fmt.Errorf("PodIP is not a valid IP address")
	}
	if o.USBIPPort < 1 || o.USBIPPort > 65535 {
		return fmt.Errorf("USBIPPort is not a valid port number")
	}

	return nil
}

func (o *usbOptions) Run(cmd *cobra.Command, _ []string) error {
	monitor, err := o.Monitor.Complete(cmd.Context(), nil)
	if err != nil {
		return err
	}

	config := usbip.USBIPDConfig{
		Port:    o.USBIPPort,
		Monitor: monitor,
	}
	err = config.Validate()
	if err != nil {
		return err
	}

	usbipd, err := config.Complete()
	if err != nil {
		return err
	}

	cfg, err := clientcmd.BuildConfigFromFlags("", o.Kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to get rest config: %w", err)
	}

	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	dynamicClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to create dynamic client: %w", err)
	}

	if err = prepare.MarkNodeForUSBGateway(cmd.Context(), o.NodeName, dynamicClient); err != nil {
		return fmt.Errorf("failed to mark node for USB gateway: %w", err)
	}

	f := informer.NewFactory(client, nil)
	resourceClaimInformer := f.ResourceClaim()
	resourceSliceInformer := f.ResourceSlice()
	nodeInformer := f.Nodes()
	podInformer := f.Pods()

	f.Start(cmd.Context().Done())
	f.WaitForCacheSync(cmd.Context().Done())

	ip := net.ParseIP(o.PodIP)
	usbIPInterface := usbip.New()
	c, err := resourceclaim.NewController(o.NodeName, ip, o.USBIPPort, client, resourceClaimInformer, resourceSliceInformer, nodeInformer, podInformer, usbIPInterface)
	if err != nil {
		return fmt.Errorf("failed to create resourceclaim controller: %w", err)
	}

	group, ctx := errgroup.WithContext(cmd.Context())
	group.Go(func() error {
		return usbipd.Run(ctx)
	})
	group.Go(func() error {
		return c.Run(ctx, 1)
	})

	return group.Wait()
}
