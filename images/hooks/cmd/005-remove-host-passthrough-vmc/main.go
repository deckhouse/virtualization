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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	removePassthroughHookName     = "Remove host-passthrough VMC"
	removePassthroughHookJQFilter = `.metadata`
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
		input.Logger.Info("No VMCs found, nothing to do")
		return nil
	}

	for _, vmc := range vmcs {
		metadata := &metav1.ObjectMeta{}
		if err := vmc.UnmarhalTo(metadata); err != nil {
			input.Logger.Error(fmt.Sprintf("Failed to unmarshal metadata vmclass %v", err))
		}
		op := "add"
		if keep, found := metadata.GetAnnotations()["helm.sh/resource-policy"]; found && keep != "keep" {
			op = "replace"
			input.Logger.Info(fmt.Sprintf("VMC %s has helm.sh/resource-policy=%s, will be replaced with helm.sh/resource-policy=keep", metadata.Name, keep))
		} else if keep == "keep" {
			input.Logger.Info(fmt.Sprintf("VMC %s already has helm.sh/resource-policy=keep", metadata.Name))
			continue
		}
		patch := []any{
			map[string]any{
				"op":    op,
				"path":  "/metadata/annotations/helm.sh~1resource-policy",
				"value": "keep",
			},
		}
		input.PatchCollector.JSONPatch(patch, "virtualization.deckhouse.io/v1alpha2", "VirtualMachineClass", "", metadata.Name)
		input.Logger.Info(fmt.Sprintf("Added helm.sh/resource-policy=keep to VirtualMachineClass %s", metadata.Name))
	}

	return nil
}

func main() {
	app.Run()
}
