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

package prevent_default_vmclasses_deletion

import (
	"context"
	"fmt"

	"hooks/pkg/settings"

	"github.com/deckhouse/virtualization/api/core"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/registry"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	removePassthroughHookName     = "Prevent default VirtualMachineClasses deletion"
	removePassthroughHookJQFilter = `.metadata`
	// see https://helm.sh/docs/howto/charts_tips_and_tricks/#tell-helm-not-to-uninstall-a-resource
	helmResourcePolicyKey  = "helm.sh/resource-policy"
	helmResourcePolicyKeep = "keep"
	apiVersion             = core.GroupName + "/" + v1alpha2.Version
)

var _ = registry.RegisterFunc(config, Reconcile)

var config = &pkg.HookConfig{
	OnBeforeHelm: &pkg.OrderedConfig{Order: 10},
	Kubernetes: []pkg.KubernetesConfig{
		{
			Name:       removePassthroughHookName,
			APIVersion: apiVersion,
			Kind:       v1alpha2.VirtualMachineClassKind,
			JqFilter:   removePassthroughHookJQFilter,

			LabelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"module": settings.ModuleName,
				},
			},
		},
	},

	Queue: fmt.Sprintf("modules/%s", settings.ModuleName),
}

func Reconcile(_ context.Context, input *pkg.HookInput) error {
	vmcs := input.Snapshots.Get(removePassthroughHookName)

	if len(vmcs) == 0 {
		input.Logger.Info("No VirtualMachineClasses found, nothing to do")
		return nil
	}

	for _, vmc := range vmcs {
		metadata := &metav1.ObjectMeta{}
		if err := vmc.UnmarshalTo(metadata); err != nil {
			input.Logger.Error(fmt.Sprintf("Failed to unmarshal metadata VirtualMachineClasses %v", err))
		}

		policy := metadata.GetAnnotations()[helmResourcePolicyKey]
		if policy == helmResourcePolicyKeep {
			input.Logger.Info(fmt.Sprintf("VirtualMachineClass %s already has helm.sh/resource-policy=keep", metadata.Name))
			continue
		}

		op := "add"
		if policy != "" {
			op = "replace"
			input.Logger.Info(fmt.Sprintf("VirtualMachineClass %s has helm.sh/resource-policy=%s, will be replaced with helm.sh/resource-policy=keep", metadata.Name, policy))
		}
		patch := []interface{}{
			map[string]string{
				"op":    op,
				"path":  "/metadata/annotations/helm.sh~1resource-policy",
				"value": helmResourcePolicyKeep,
			},
		}
		input.PatchCollector.JSONPatch(patch, apiVersion, v1alpha2.VirtualMachineClassKind, "", metadata.Name)
		input.Logger.Info(fmt.Sprintf("Added helm.sh/resource-policy=keep to VirtualMachineClass %s", metadata.Name))
	}

	return nil
}
