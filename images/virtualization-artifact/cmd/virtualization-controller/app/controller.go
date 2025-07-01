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

	"github.com/deckhouse/deckhouse/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/deckhouse/virtualization-controller/pkg/config"
	"github.com/deckhouse/virtualization-controller/pkg/config/apis/componentconfig"
	"github.com/deckhouse/virtualization-controller/pkg/controller/cvi"
	"github.com/deckhouse/virtualization-controller/pkg/controller/evacuation"
	"github.com/deckhouse/virtualization-controller/pkg/controller/livemigration"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vd"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vdsnapshot"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vi"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmbda"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmclass"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmip"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmiplease"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmrestore"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmsnapshot"
	workloadupdater "github.com/deckhouse/virtualization-controller/pkg/controller/workload-updater"
	"github.com/deckhouse/virtualization/api/client/kubeclient"
)

var controllers = map[string]func(
	ctx context.Context,
	mgr manager.Manager,
	log *log.Logger,
	configuration *componentconfig.VirtualizationControllerConfiguration,
	virtualizationClient kubeclient.Client,
) error{
	cvi.ControllerName: func(ctx context.Context, mgr manager.Manager, log *log.Logger, configuration *componentconfig.VirtualizationControllerConfiguration, virtualizationClient kubeclient.Client) error {
		_, err := cvi.NewController(ctx,
			mgr,
			log,
			configuration.Spec.ImportSettings.ImporterImage,
			configuration.Spec.ImportSettings.UploaderImage,
			configuration.Spec.ImportSettings.Requirements,
			config.ToLegacyDVCR(configuration),
			configuration.Spec.Namespace)
		return err
	},
	vd.ControllerName: func(ctx context.Context, mgr manager.Manager, log *log.Logger, configuration *componentconfig.VirtualizationControllerConfiguration, virtualizationClient kubeclient.Client) error {
		_, err := vd.NewController(
			ctx,
			mgr,
			log,
			configuration.Spec.ImportSettings.ImporterImage,
			configuration.Spec.ImportSettings.UploaderImage,
			configuration.Spec.ImportSettings.Requirements,
			config.ToLegacyDVCR(configuration),
			config.ToLegacyVirtualDiskStorageClassSettings(configuration))
		return err
	},
	vi.ControllerName: func(ctx context.Context, mgr manager.Manager, log *log.Logger, configuration *componentconfig.VirtualizationControllerConfiguration, virtualizationClient kubeclient.Client) error {
		_, err := vi.NewController(
			ctx,
			mgr,
			log,
			configuration.Spec.ImportSettings.ImporterImage,
			configuration.Spec.ImportSettings.UploaderImage,
			configuration.Spec.ImportSettings.BounderImage,
			configuration.Spec.ImportSettings.Requirements,
			config.ToLegacyDVCR(configuration),
			config.ToLegacyVirtualImageStorageClassSettings(configuration))
		return err
	},
	vm.ControllerName: func(ctx context.Context, mgr manager.Manager, log *log.Logger, configuration *componentconfig.VirtualizationControllerConfiguration, virtualizationClient kubeclient.Client) error {
		return vm.SetupController(
			ctx,
			mgr,
			log,
			config.ToLegacyDVCR(configuration),
			configuration.Spec.FirmwareImage)
	},
	vm.GCVMMigrationControllerName: func(ctx context.Context, mgr manager.Manager, log *log.Logger, configuration *componentconfig.VirtualizationControllerConfiguration, virtualizationClient kubeclient.Client) error {
		return vm.SetupGC(mgr, log, configuration.Spec.GarbageCollector.VMIMigration)
	},
	vmbda.ControllerName: func(ctx context.Context, mgr manager.Manager, log *log.Logger, configuration *componentconfig.VirtualizationControllerConfiguration, virtualizationClient kubeclient.Client) error {
		_, err := vmbda.NewController(ctx, mgr, virtualizationClient, log, configuration.Spec.Namespace)
		return err
	},
	vmip.ControllerName: func(ctx context.Context, mgr manager.Manager, log *log.Logger, configuration *componentconfig.VirtualizationControllerConfiguration, virtualizationClient kubeclient.Client) error {
		_, err := vmip.NewController(ctx, mgr, virtualizationClient, log, configuration.Spec.VirtualMachineCIDRs)
		return err
	},
	vmiplease.ControllerName: func(ctx context.Context, mgr manager.Manager, log *log.Logger, configuration *componentconfig.VirtualizationControllerConfiguration, virtualizationClient kubeclient.Client) error {
		_, err := vmiplease.NewController(ctx, mgr, log, configuration.Spec.VirtualMachineIPLeasesRetentionDuration)
		return err
	},
	vmclass.ControllerName: func(ctx context.Context, mgr manager.Manager, log *log.Logger, configuration *componentconfig.VirtualizationControllerConfiguration, virtualizationClient kubeclient.Client) error {
		_, err := vmclass.NewController(ctx, mgr, configuration.Spec.Namespace, log)
		return err
	},
	vdsnapshot.ControllerName: func(ctx context.Context, mgr manager.Manager, log *log.Logger, configuration *componentconfig.VirtualizationControllerConfiguration, virtualizationClient kubeclient.Client) error {
		_, err := vdsnapshot.NewController(ctx, mgr, log, virtualizationClient)
		return err
	},
	vmsnapshot.ControllerName: func(ctx context.Context, mgr manager.Manager, log *log.Logger, configuration *componentconfig.VirtualizationControllerConfiguration, virtualizationClient kubeclient.Client) error {
		return vmsnapshot.NewController(ctx, mgr, log, virtualizationClient)
	},
	vmrestore.ControllerName: func(ctx context.Context, mgr manager.Manager, log *log.Logger, configuration *componentconfig.VirtualizationControllerConfiguration, virtualizationClient kubeclient.Client) error {
		return vmrestore.NewController(ctx, mgr, log)
	},
	vmop.ControllerName: func(ctx context.Context, mgr manager.Manager, log *log.Logger, configuration *componentconfig.VirtualizationControllerConfiguration, virtualizationClient kubeclient.Client) error {
		return vmop.SetupController(ctx, mgr, log)
	},
	vmop.GCControllerName: func(ctx context.Context, mgr manager.Manager, log *log.Logger, configuration *componentconfig.VirtualizationControllerConfiguration, virtualizationClient kubeclient.Client) error {
		return vmop.SetupGC(mgr, log, configuration.Spec.GarbageCollector.VMOP)
	},
	livemigration.ControllerName: func(ctx context.Context, mgr manager.Manager, log *log.Logger, configuration *componentconfig.VirtualizationControllerConfiguration, virtualizationClient kubeclient.Client) error {
		return livemigration.SetupController(ctx, mgr, log)
	},
	workloadupdater.ControllerName: func(ctx context.Context, mgr manager.Manager, log *log.Logger, configuration *componentconfig.VirtualizationControllerConfiguration, virtualizationClient kubeclient.Client) error {
		return workloadupdater.SetupController(ctx, mgr, log, configuration.Spec.FirmwareImage, configuration.Spec.Namespace, configuration.Spec.VirtControllerName)
	},
	evacuation.ControllerName: func(ctx context.Context, mgr manager.Manager, log *log.Logger, configuration *componentconfig.VirtualizationControllerConfiguration, virtualizationClient kubeclient.Client) error {
		return evacuation.SetupController(ctx, mgr, virtualizationClient, log)
	},
}
