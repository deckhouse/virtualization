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

package migrate_vm_network_interface_ids

import (
	"context"
	"fmt"

	"k8s.io/utils/ptr"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/registry"
	"github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/hooks/pkg/settings"
)

const (
	virtualMachinesSnapshot = "virtual-machines"
	vmJQFilter              = `{name: .metadata.name, namespace: .metadata.namespace, networks: .spec.networks}`
)

type virtualMachineSnapshot struct {
	Name      string                  `json:"name"`
	Namespace string                  `json:"namespace"`
	Networks  []v1alpha2.NetworksSpec `json:"networks"`
}

var _ = registry.RegisterFunc(config, handler)

var config = &pkg.HookConfig{
	OnBeforeHelm: &pkg.OrderedConfig{Order: 5},
	Kubernetes: []pkg.KubernetesConfig{
		{
			Name:                         virtualMachinesSnapshot,
			APIVersion:                   v1alpha2.SchemeGroupVersion.String(),
			Kind:                         v1alpha2.VirtualMachineKind,
			JqFilter:                     vmJQFilter,
			ExecuteHookOnSynchronization: ptr.To(false),
			ExecuteHookOnEvents:          ptr.To(false),
		},
	},

	Queue: fmt.Sprintf("modules/%s", settings.ModuleName),
}

func handler(_ context.Context, input *pkg.HookInput) error {
	vms := input.Snapshots.Get(virtualMachinesSnapshot)
	if len(vms) == 0 {
		return nil
	}

	for _, vm := range vms {
		var vmSnap virtualMachineSnapshot
		if err := vm.UnmarshalTo(&vmSnap); err != nil {
			input.Logger.Error(fmt.Sprintf("Failed to unmarshal VM snapshot: %v", err))
			continue
		}

		if !network.EnsureNetworkInterfaceIDs(vmSnap.Networks) {
			continue
		}

		patch := []map[string]any{
			{
				"op":    "add",
				"path":  "/spec/networks",
				"value": vmSnap.Networks,
			},
		}

		input.PatchCollector.PatchWithJSON(
			patch,
			v1alpha2.SchemeGroupVersion.String(),
			v1alpha2.VirtualMachineKind,
			vmSnap.Namespace,
			vmSnap.Name,
		)
		input.Logger.Info(fmt.Sprintf("Patched VirtualMachine %s/%s with missing network interface IDs", vmSnap.Namespace, vmSnap.Name))
	}

	return nil
}
