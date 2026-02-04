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

	"github.com/spf13/cobra"
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
	"github.com/deckhouse/virtualization-dra/internal/usb-gateway/prepare"
	"github.com/deckhouse/virtualization-dra/pkg/controller"
	"github.com/deckhouse/virtualization-dra/pkg/libusb"
	"github.com/deckhouse/virtualization-dra/pkg/logger"
	"github.com/deckhouse/virtualization-dra/pkg/usbip"
)

func NewVirtualizationDraPluginCommand() *cobra.Command {
	o := newDraOptions()

	cmd := &cobra.Command{
		Use:           "virtualization-dra-plugin",
		Short:         "virtualization-dra-plugin",
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
		Kubeconfig:                   os.Getenv("KUBECONFIG"),
		NodeName:                     os.Getenv("NODE_NAME"),
		USBGatewaySecretName:         "virtualization-dra-usb-gateway",
		CDIRoot:                      withDefault("CDI_ROOT", cdi.SpecDir),
		KubeletRegisterDirectoryPath: os.Getenv("KUBELET_REGISTER_DIRECTORY_PATH"),
		KubeletPluginsDirectoryPath:  os.Getenv("KUBELET_PLUGINS_DIRECTORY_PATH"),
		HealthzPort:                  51515,
		logging:                      &logger.Options{},
		monitor:                      libusb.NewDefaultMonitorConfig(),
		usbipdConfig:                 &usbip.USBIPDConfig{},
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
	Namespace                    string
	NodeName                     string
	USBGatewaySecretName         string
	CDIRoot                      string
	KubeletRegisterDirectoryPath string
	KubeletPluginsDirectoryPath  string
	HealthzPort                  int

	logging      *logger.Options
	monitor      *libusb.MonitorConfig
	usbipdConfig *usbip.USBIPDConfig

	featureGates featuregates.AddFlagsFunc

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
	mfs := fs.FlagSet("virtualization-dra plugin")
	mfs.StringVar(&o.Kubeconfig, "kubeconfig", o.Kubeconfig, "Path to kubeconfig file")
	mfs.StringVar(&o.Namespace, "namespace", o.Namespace, "Namespace")
	mfs.StringVar(&o.NodeName, "node-name", o.NodeName, "Node name")
	mfs.StringVar(&o.USBGatewaySecretName, "usb-gateway-secret-name", o.USBGatewaySecretName, "USB gateway secret name")
	mfs.StringVar(&o.CDIRoot, "cdi-root", o.CDIRoot, "CDI root")
	mfs.StringVar(&o.KubeletRegisterDirectoryPath, "kubelet-register-directory-path", o.KubeletRegisterDirectoryPath, "Kubelet register directory path")
	mfs.StringVar(&o.KubeletPluginsDirectoryPath, "kubelet-plugins-directory-path", o.KubeletPluginsDirectoryPath, "Kubelet plugins directory path")
	mfs.IntVar(&o.HealthzPort, "healthz-port", o.HealthzPort, "Healthz port")

	o.logging.AddFlags(fs.FlagSet("logging"))

	o.monitor.AddFlags(fs.FlagSet("usb-monitor"))

	o.usbipdConfig.AddFlags(fs.FlagSet("usbipd"))

	o.featureGates(fs.FlagSet("feature-gates"))

	return fs
}

func (o *draOptions) Validate() error {
	if o.Namespace == "" {
		return fmt.Errorf("Namespace is required")
	}
	if o.NodeName == "" {
		return fmt.Errorf("NodeName is required")
	}
	if o.CDIRoot == "" {
		return fmt.Errorf("CDIRoot is required")
	}
	if o.HealthzPort <= 0 {
		return fmt.Errorf("HealthzPort is required")
	}

	if o.usbGatewayEnabled {
		if o.USBGatewaySecretName == "" {
			return fmt.Errorf("USBGatewaySecretName is required")
		}
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
	err := plugin.InitPluginDirs(o.KubeletPluginsDirectoryPath, o.KubeletRegisterDirectoryPath)
	if err != nil {
		return err
	}

	client, dynamicClient, err := o.Clients()
	if err != nil {
		return err
	}
	_ = dynamicClient

	monitor, err := o.monitor.Complete(cmd.Context(), nil)
	if err != nil {
		return fmt.Errorf("failed to create USB monitor: %w", err)
	}

	var usbGateway usbgateway.USBGateway

	if o.usbGatewayEnabled {
		usbipd, err := o.usbipdConfig.Complete(monitor)
		if err != nil {
			return fmt.Errorf("failed to create USBIPD: %w", err)
		}

		f := informer.NewFactory(client, nil)
		f.WaitForCacheSync(cmd.Context().Done())

		secretInformer := f.NamespacedSecret(o.Namespace)
		resourceSliceInformer := f.ResourceSlice()

		usbGatewayController, err := usbgateway.NewUSBGatewayController(
			cmd.Context(),
			o.USBGatewaySecretName,
			o.Namespace,
			o.NodeName,
			o.usbipdConfig.Address,
			o.usbipdConfig.Port,
			client,
			secretInformer,
			resourceSliceInformer,
			usbip.New(),
		)
		if err != nil {
			return fmt.Errorf("failed to create USB gateway controller: %w", err)
		}

		if err = usbipd.Run(cmd.Context()); err != nil {
			return fmt.Errorf("failed to run USBIPD: %w", err)
		}
		if err = controller.Run(usbGatewayController, cmd.Context(), 1); err != nil {
			return fmt.Errorf("failed to run USB gateway controller: %w", err)
		}

		err = prepare.MarkNodeForUSBGateway(cmd.Context(), o.NodeName, dynamicClient)
		if err != nil {
			return fmt.Errorf("failed to mark node for USB gateway: %w", err)
		}
		defer func() {
			err = prepare.UnmarkNodeForUSBGateway(cmd.Context(), o.NodeName, dynamicClient)
			if err != nil {
				slog.Error("failed to unmark node for USB gateway", slog.Any("error", err))
			}
		}()

		usbGateway = usbGatewayController
	}

	usbCDIManager, err := cdi.NewCDIManager(o.CDIRoot, "usb", plugin.DriverName, o.NodeName, "DRA_USB")
	if err != nil {
		return fmt.Errorf("failed to create CDI manager: %w", err)
	}

	usbStore, err := usb.NewAllocationStore(cmd.Context(), o.NodeName, usbCDIManager, monitor, usbGateway, slog.Default())
	if err != nil {
		return fmt.Errorf("failed to create USB store: %w", err)
	}

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

	driver.Wait()
	driver.Shutdown()
	healthCheck.Stop()

	return nil
}
