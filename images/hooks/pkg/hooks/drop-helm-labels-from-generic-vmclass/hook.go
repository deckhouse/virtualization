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

package drop_helm_labels_from_generic_vmclass

import (
	"context"
	"fmt"
	"strings"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/registry"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"hooks/pkg/settings"
)

const (
	vmClassSnapshot    = "vmclass-generic"
	genericVMClassName = "generic"
)

const (
	helmManagedByLabel = "app.kubernetes.io/managed-by"
	helmHeritageLabel  = "heritage"
)

var _ = registry.RegisterFunc(configDropHelmLabels, handlerDropHelmLabels)

var configDropHelmLabels = &pkg.HookConfig{
	OnAfterHelm: &pkg.OrderedConfig{Order: 10},
	Kubernetes: []pkg.KubernetesConfig{
		{
			Name:       vmClassSnapshot,
			APIVersion: "deckhouse.io/v1alpha2",
			Kind:       "VirtualMachineClass",
			JqFilter:   ".metadata",
			NameSelector: &pkg.NameSelector{
				MatchNames: []string{genericVMClassName},
			},
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":                          "virtualization-controller",
					"app.kubernetes.io/managed-by": "Helm",
					"heritage":                     "deckhouse",
					"module":                       settings.ModuleName,
				},
			},
			ExecuteHookOnEvents: ptr.To(false),
		},
	},

	Queue: fmt.Sprintf("modules/%s", settings.ModuleName),
}

type VMClassMetadata struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels"`
}

func handlerDropHelmLabels(_ context.Context, input *pkg.HookInput) error {
	snaps := input.Snapshots.Get(vmClassSnapshot)
	if len(snaps) == 0 {
		return nil
	}

	vmClass := &VMClassMetadata{}
	err := snaps[0].UnmarshalTo(vmClass)
	if err != nil {
		input.Logger.Error("failed to unmarshal VMClass", "error", err)
		return err
	}

	if vmClass.Labels == nil {
		return nil
	}

	// Check if VMClass has all required labels to be processed
	if vmClass.Labels["app"] != "virtualization-controller" ||
		vmClass.Labels["module"] != settings.ModuleName ||
		vmClass.Labels[helmManagedByLabel] != "Helm" ||
		vmClass.Labels[helmHeritageLabel] != "deckhouse" {
		input.Logger.Debug("VMClass doesn't match required labels, skipping")
		return nil
	}

	var patches []map[string]interface{}
	hasLabelsToRemove := false

	// Check and prepare patches for Helm labels
	if _, exists := vmClass.Labels[helmManagedByLabel]; exists {
		patches = append(patches, map[string]interface{}{
			"op":    "remove",
			"path":  fmt.Sprintf("/metadata/labels/%s", jsonPatchEscape(helmManagedByLabel)),
			"value": nil,
		})
		hasLabelsToRemove = true
	}

	if _, exists := vmClass.Labels[helmHeritageLabel]; exists {
		patches = append(patches, map[string]interface{}{
			"op":    "remove",
			"path":  fmt.Sprintf("/metadata/labels/%s", jsonPatchEscape(helmHeritageLabel)),
			"value": nil,
		})
		hasLabelsToRemove = true
	}

	if !hasLabelsToRemove {
		return nil
	}

	input.Logger.Info("Removing Helm labels from generic VMClass")
	input.PatchCollector.PatchWithJSON(
		patches,
		"deckhouse.io/v1alpha2",
		"VirtualMachineClass",
		"",
		genericVMClassName,
	)

	return nil
}

func jsonPatchEscape(s string) string {
	return strings.NewReplacer("~", "~0", "/", "~1").Replace(s)
}
