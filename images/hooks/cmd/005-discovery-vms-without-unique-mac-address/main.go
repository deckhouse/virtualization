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

package main

import (
	"context"
	"fmt"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/app"
	"github.com/deckhouse/module-sdk/pkg/registry"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"

	"hooks/pkg/common"
)

const (
	macAnnotation = "cni.cilium.io/macAddress"
	configName    = "vm-without-uniue-mac-address"
)

var _ = registry.RegisterFunc(configAllVirtualMachines, handlerVirtualMachines)

var configAllVirtualMachines = &pkg.HookConfig{
	OnBeforeHelm: &pkg.OrderedConfig{Order: 5},
	Kubernetes: []pkg.KubernetesConfig{
		{
			Name:                         configName,
			APIVersion:                   "virtualization.deckhouse.io/v1alpha2",
			Kind:                         "VirtualMachine",
			JqFilter:                     "",
			ExecuteHookOnSynchronization: ptr.To(false),
			NameSelector: &pkg.NameSelector{
				MatchNames: []string{},
			},
			NamespaceSelector: &pkg.NamespaceSelector{
				NameSelector: &pkg.NameSelector{
					MatchNames: []string{},
				},
			},
		},
	},
	Queue: fmt.Sprintf("modules/%s", common.MODULE_NAME),
}

func handlerVirtualMachines(_ context.Context, input *pkg.HookInput) error {
	vmSnapshots := input.Snapshots.Get(configName)

	if len(vmSnapshots) == 0 {
		return nil
	}

	ctx := context.TODO()
	k8sClient, err := input.DC.GetK8sClient()
	if err != nil {
		return fmt.Errorf("error obtaining Kubernetes client: %v", err)
	}

	for _, vmSnapshot := range vmSnapshots {
		var vm virtv2.VirtualMachine
		err = vmSnapshot.UnmarhalTo(&vm)
		if err != nil {
			return fmt.Errorf("unmarshalTo: %w", err)
		}

		pod := &corev1.Pod{}
		err = k8sClient.Get(ctx, client.ObjectKey{
			Namespace: vm.Namespace,
			Name:      vm.Status.VirtualMachinePods[0].Name,
		}, pod)
		if err != nil {
			return fmt.Errorf("error fetching Pod: %v", err)
		}

		if pod.Annotations[macAnnotation] == "" {
			input.PatchCollector.MergePatch(map[string]interface{}{
				"metadata": map[string]interface{}{
					"annotations": map[string]string{
						"virtualization.deckhouse.io/migration-to-mac-addresses": "false",
					},
				},
			}, "virtualization.deckhouse.io/v1alpha2", "VirtualMachine", vm.Namespace, vm.Name)
		}
	}

	return nil
}

func main() {
	app.Run()
}
