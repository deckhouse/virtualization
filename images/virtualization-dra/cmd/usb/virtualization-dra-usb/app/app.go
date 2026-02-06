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
	"os"
	"strconv"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/component-base/cli/flag"

	"github.com/deckhouse/virtualization-dra/internal/cdi"
	"github.com/deckhouse/virtualization-dra/internal/plugin"
	"github.com/deckhouse/virtualization-dra/internal/usb"
	"github.com/deckhouse/virtualization-dra/pkg/libusb"
	"github.com/deckhouse/virtualization-dra/pkg/logger"
)

func NewVirtualizationDraUSBCommand() *cobra.Command {
	o := newDraOptions()

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

	return cmd
}

func newDraOptions() *draOptions {
	withDefault := func(env, defaultValue string) string {
		if env, ok := os.LookupEnv(env); ok {
			return env
		}
		return defaultValue
	}

	o := &draOptions{
		DriverName:  usb.DriverName,
		Kubeconfig:  os.Getenv("KUBECONFIG"),
		NodeName:    os.Getenv("NODE_NAME"),
		Namespace:   os.Getenv("NAMESPACE"),
		CDIRoot:     withDefault("CDI_ROOT", cdi.SpecDir),
		HealthzPort: 51515,
		logging:     &logger.Options{},
		monitor:     libusb.NewDefaultMonitorConfig(),
	}

	if healthzPort := os.Getenv("HEALTHZ_PORT"); healthzPort != "" {
		port, err := strconv.Atoi(healthzPort)
		if err == nil {
			o.HealthzPort = port
		}
	}

	return o
}

type draOptions struct {
	DriverName  string
	Kubeconfig  string
	Namespace   string
	NodeName    string
	CDIRoot     string
	HealthzPort int

	logging *logger.Options
	monitor *libusb.MonitorConfig
}

func (o *draOptions) Complete() {
	log := o.logging.Complete()
	logger.SetDefaultLogger(log)
}

func (o *draOptions) NamedFlags() (fs flag.NamedFlagSets) {
	mfs := fs.FlagSet("virtualization-usb plugin")
	mfs.StringVar(&o.DriverName, "driver-name", o.DriverName, "Driver name")
	mfs.StringVar(&o.Kubeconfig, "kubeconfig", o.Kubeconfig, "Path to kubeconfig file")
	mfs.StringVar(&o.Namespace, "namespace", o.Namespace, "Namespace")
	mfs.StringVar(&o.NodeName, "node-name", o.NodeName, "Node name")
	mfs.StringVar(&o.CDIRoot, "cdi-root", o.CDIRoot, "CDI root")
	mfs.IntVar(&o.HealthzPort, "healthz-port", o.HealthzPort, "Healthz port")

	o.logging.AddFlags(fs.FlagSet("logging"))
	o.monitor.AddFlags(fs.FlagSet("usb-monitor"))
	plugin.AddFlags(fs.FlagSet("plugin"))

	return fs
}

func (o *draOptions) Validate() error {
	if o.Namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	if o.NodeName == "" {
		return fmt.Errorf("nodeName is required")
	}
	if o.CDIRoot == "" {
		return fmt.Errorf("cdiRoot is required")
	}
	if o.HealthzPort <= 0 {
		return fmt.Errorf("healthzPort is required")
	}

	return nil
}

func (o *draOptions) Client() (kubernetes.Interface, error) {
	cfg, err := clientcmd.BuildConfigFromFlags("", o.Kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to get rest config: %w", err)
	}

	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	return client, nil
}

func (o *draOptions) Run(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()

	client, err := o.Client()
	if err != nil {
		return err
	}

	monitor, err := o.monitor.Complete(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to create USB monitor: %w", err)
	}

	group, ctx := errgroup.WithContext(ctx)

	usbCDIManager, err := cdi.NewCDIManager(o.CDIRoot, "usb", o.DriverName, o.NodeName, "DRA_USB")
	if err != nil {
		return fmt.Errorf("failed to create CDI manager: %w", err)
	}

	usbStore, err := usb.NewAllocationStore(ctx, o.NodeName, usbCDIManager, monitor)
	if err != nil {
		return fmt.Errorf("failed to create USB store: %w", err)
	}

	mgr, err := plugin.NewManager(o.DriverName, o.NodeName, client, usbStore, o.HealthzPort, false)
	if err != nil {
		return fmt.Errorf("failed to create manager: %w", err)
	}

	group.Go(func() error {
		return mgr.Run(ctx)
	})

	return group.Wait()
}
