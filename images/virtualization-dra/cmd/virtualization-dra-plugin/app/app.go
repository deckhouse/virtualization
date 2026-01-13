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
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/component-base/cli/flag"

	"github.com/deckhouse/virtualization-dra/internal/cdi"
	"github.com/deckhouse/virtualization-dra/internal/featuregates"
	"github.com/deckhouse/virtualization-dra/internal/plugin"
	"github.com/deckhouse/virtualization-dra/internal/usb"
	"github.com/deckhouse/virtualization-dra/pkg/logger"
)

func NewVirtualizationDraPluginCommand() *cobra.Command {
	o := newDraOptions()

	cmd := &cobra.Command{
		Use:           "virtualization-dra-plugin",
		Short:         "virtualization-dra-plugin",
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
		Kubeconfig:                   os.Getenv("KUBECONFIG"),
		NodeName:                     os.Getenv("NODE_NAME"),
		CDIRoot:                      withDefault("CDI_ROOT", cdi.SpecDir),
		KubeletRegisterDirectoryPath: os.Getenv("KUBELET_REGISTER_DIRECTORY_PATH"),
		KubeletPluginsDirectoryPath:  os.Getenv("KUBELET_PLUGINS_DIRECTORY_PATH"),
		USBDevicesPath:               withDefault("USB_DEVICES_PATH", usb.PathToUSBDevices),
		HealthzPort:                  51515,
		USBResyncPeriod:              usb.DefaultResyncPeriod,
		Logging:                      &logger.Options{},
		featureGates:                 featuregates.AddFlags,
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
	Kubeconfig                   string
	NodeName                     string
	CDIRoot                      string
	KubeletRegisterDirectoryPath string
	KubeletPluginsDirectoryPath  string
	USBDevicesPath               string
	HealthzPort                  int
	USBResyncPeriod              time.Duration

	Logging      *logger.Options
	featureGates featuregates.AddFlagsFunc
}

func (o *draOptions) NamedFlags() (fs flag.NamedFlagSets) {
	mfs := fs.FlagSet("virtualization-dra plugin")
	mfs.StringVar(&o.Kubeconfig, "kubeconfig", o.Kubeconfig, "Path to kubeconfig file")
	mfs.StringVar(&o.NodeName, "node-name", o.NodeName, "Node name")
	mfs.StringVar(&o.CDIRoot, "cdi-root", o.CDIRoot, "CDI root")
	mfs.StringVar(&o.KubeletRegisterDirectoryPath, "kubelet-register-directory-path", o.KubeletRegisterDirectoryPath, "Kubelet register directory path")
	mfs.StringVar(&o.KubeletPluginsDirectoryPath, "kubelet-plugins-directory-path", o.KubeletPluginsDirectoryPath, "Kubelet plugins directory path")
	mfs.StringVar(&o.USBDevicesPath, "usb-devices-path", o.USBDevicesPath, "USB Devices path")
	mfs.IntVar(&o.HealthzPort, "healthz-port", o.HealthzPort, "Healthz port")
	mfs.DurationVar(&o.USBResyncPeriod, "usb-resync-period", o.USBResyncPeriod, "USB resync period")

	o.Logging.AddFlags(fs.FlagSet("logging"))

	o.featureGates(fs.FlagSet("feature-gates"))

	return fs
}

func (o *draOptions) Validate() error {
	if o.NodeName == "" {
		return fmt.Errorf("NodeName is required")
	}
	if o.CDIRoot == "" {
		return fmt.Errorf("CDIRoot is required")
	}
	if o.HealthzPort <= 0 {
		return fmt.Errorf("HealthzPort is required")
	}

	return nil
}

func (o *draOptions) Run(cmd *cobra.Command, _ []string) error {
	err := plugin.InitPluginDirs(o.KubeletPluginsDirectoryPath, o.KubeletRegisterDirectoryPath)
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

	usbCDIManager, err := cdi.NewCDIManager(o.CDIRoot, "usb", plugin.DriverName, o.NodeName, "DRA_USB")
	if err != nil {
		return fmt.Errorf("failed to create CDI manager: %w", err)
	}

	usbStore := usb.NewAllocationStore(o.NodeName, o.USBDevicesPath, o.USBResyncPeriod, usbCDIManager, slog.Default())

	driver := plugin.NewDriver(o.NodeName, client, usbStore, slog.Default())
	err = driver.Start(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to start driver: %w", err)
	}

	healthCheck := plugin.NewHealthCheck(o.HealthzPort, slog.Default())
	err = healthCheck.Start()
	if err != nil {
		return fmt.Errorf("failed to start health check: %w", err)
	}

	err = usbStore.Start(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to start usb store: %w", err)
	}

	driver.Wait()
	driver.Shutdown()
	healthCheck.Stop()

	return nil
}
