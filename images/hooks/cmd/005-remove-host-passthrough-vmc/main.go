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
	"hooks/pkg/common"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/app"
	"github.com/deckhouse/module-sdk/pkg/registry"
	// "k8s.io/utils/ptr"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	removePassthroughHookName     = "Remove host-passthrough VMC"
	removePassthroughHookJQFilter = `.metadata.name`
)

var _ = registry.RegisterFunc(config, handler)

var config = &pkg.HookConfig{
	OnBeforeHelm: &pkg.OrderedConfig{Order: 10},
	Kubernetes: []pkg.KubernetesConfig{
		{
			Name:       removePassthroughHookName,
			APIVersion: "virtualization.deckhouse.io/v1alpha2",
			Kind:       "VirtualMachineClass",
			JqFilter:   removePassthroughHookJQFilter,

			// ExecuteHookOnSynchronization: ptr.To(false),

			NamespaceSelector: &pkg.NamespaceSelector{
				NameSelector: &pkg.NameSelector{
					MatchNames: []string{common.MODULE_NAMESPACE},
				},
			},
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"module": common.MODULE_NAME,
				},
			},
		},
	},

	Queue: fmt.Sprintf("modules/%s", common.MODULE_NAME),
}

func handler(_ context.Context, input *pkg.HookInput) error {
	vmcs := input.Snapshots.Get(removePassthroughHookName)

	if len(vmcs) == 0 {
		input.Logger.Info(fmt.Sprintf("No VMCs found in namespace %s, nothing to do", common.MODULE_NAMESPACE))
		return nil
	}

	for _, vmc := range vmcs {
		name := vmc.String() 
		patch := []any{
			map[string]any{
				"op":    "add",
				"path":  "/metadata/annotations/helm.sh~1resource-policy",
				"value": "keep",
			},
		}
		input.PatchCollector.JSONPatch(patch, "virtualization.deckhouse.io/v1alpha2", "VirtualMachineClass", common.MODULE_NAMESPACE, name)
		input.Logger.Info(fmt.Sprintf("Added helm.sh/resource-policy=keep to VirtualMachineClass %s", name))
	}

	return nil
}

func main() {
	app.Run()
}
