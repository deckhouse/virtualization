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

package install_vmclass_generic

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"hooks/pkg/settings"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/registry"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

const (
	moduleStateSecretSnapshot = "module-state-snapshot"
	moduleStateSecretName     = "module-state"

	vmClassGenericSnapshot = "vmclass-generic-snapshot"
	vmClassGenericName     = "generic"

	vmClassInstallationStateSecretKey  = "vmClassGenericInstallation"
	vmClassInstallationStateValuesPath = "virtualization.internal.moduleState." + vmClassInstallationStateSecretKey
)

var _ = registry.RegisterFunc(config, Reconcile)

// This hook runs before applying templates (OnBeforeHelm) to drop helm labels
// and make vmclass unmanageable.
var config = &pkg.HookConfig{
	OnBeforeHelm: &pkg.OrderedConfig{Order: 5},
	Kubernetes: []pkg.KubernetesConfig{
		{
			Name:       moduleStateSecretSnapshot,
			APIVersion: "v1",
			Kind:       "Secret",
			JqFilter:   `{data}`,
			NameSelector: &pkg.NameSelector{
				MatchNames: []string{moduleStateSecretName},
			},
			NamespaceSelector: &pkg.NamespaceSelector{
				NameSelector: &pkg.NameSelector{
					MatchNames: []string{settings.ModuleNamespace},
				},
			},
			ExecuteHookOnSynchronization: ptr.To(false),
			ExecuteHookOnEvents:          ptr.To(false),
		},
		{
			Name:     vmClassGenericSnapshot,
			Kind:     v1alpha2.VirtualMachineClassKind,
			JqFilter: `{apiVersion, kind, "metadata": ( .metadata | {name, labels, annotations, creationTimestamp} ) }`,
			NameSelector: &pkg.NameSelector{
				MatchNames: []string{vmClassGenericName},
			},
			ExecuteHookOnSynchronization: ptr.To(false),
			ExecuteHookOnEvents:          ptr.To(false),
		},
	},

	Queue: fmt.Sprintf("modules/%s", settings.ModuleName),
}

// Reconcile manages the state of vmclass/generic resource:
//
// - Install a new one if there is no state in the Secret indicating that the vmclass was installed earlier.
// - Removes helm related annotations and labels from existing vmclass/generic (one time operation).
// - No actions performed if user deletes or replaces vmclass/generic.
func Reconcile(_ context.Context, input *pkg.HookInput) error {
	moduleState, err := parseVMClassInstallationStateFromSnapshot(input)
	if err != nil {
		return err
	}

	// If there is a state for vmclass/generic in the Secret, no changes to vmclass is required.
	// Presence of the vmclass is not important, user may delete it and it's ok.
	// The important part is to copy state from the Secret into values
	// to ensure correct manifest for the Secret template (there may be no state in values, e.g. after deckhouse restart).
	if moduleState != nil {
		input.Values.Set(vmClassInstallationStateValuesPath, vmClassInstallationState{InstalledAt: moduleState.InstalledAt})
		return nil
	}

	// Corner case: the secret is gone, but the state is present in values.
	// Just return without changes to vmclass/generic, so helm will re-create
	// the Secret with the module state.
	stateInValues := input.Values.Get(vmClassInstallationStateValuesPath)
	if stateInValues.Exists() {
		return nil
	}

	vmClassGeneric, err := parseVMClassGenericFromSnapshot(input)
	if err != nil {
		return err
	}

	// No state in secret, no state in values, no vmclass/generic.
	// Create vmclass/generic and set state in values, as it should be initial module installation.
	if vmClassGeneric == nil {
		input.Logger.Info("Install VirtualMachineClass/generic")
		vmClass := vmClassGenericManifest()
		input.PatchCollector.Create(vmClass)
	}
	// No state in secret, no state in values, but vmclass/generic is present.
	// Cleanup metadata if vmclass was created by earlier versions of the module.
	if isManagedByModule(vmClassGeneric) {
		addPatchesToCleanupMetadata(input, vmClassGeneric)
	}

	// Set state in values to prevent any further updates to vmclass/generic.
	input.Values.Set(vmClassInstallationStateValuesPath, vmClassInstallationState{InstalledAt: time.Now()})
	return nil
}

type vmClassInstallationState struct {
	InstalledAt time.Time `json:"installedAt"`
}

// parseVMClassInstallationStateFromSnapshot unmarshal vmClassInstallationState from jqFilter result.
func parseVMClassInstallationStateFromSnapshot(input *pkg.HookInput) (*vmClassInstallationState, error) {
	snap := input.Snapshots.Get(moduleStateSecretSnapshot)
	if len(snap) < 1 {
		return nil, nil
	}

	var ms corev1.Secret
	err := snap[0].UnmarshalTo(&ms)
	if err != nil {
		return nil, err
	}

	stateRaw := ms.Data[vmClassInstallationStateSecretKey]
	if len(stateRaw) == 0 {
		return nil, nil
	}

	var s vmClassInstallationState
	err = json.Unmarshal(stateRaw, &s)
	if err != nil {
		return nil, fmt.Errorf("restore vmclass generic state from secret: %w", err)
	}

	return &s, nil
}

// parseVMClassGenericFromSnapshot unmarshal ModuleConfig from jqFilter result.
func parseVMClassGenericFromSnapshot(input *pkg.HookInput) (*v1alpha2.VirtualMachineClass, error) {
	snap := input.Snapshots.Get(vmClassGenericSnapshot)
	if len(snap) < 1 {
		return nil, nil
	}

	var vmclass v1alpha2.VirtualMachineClass
	err := snap[0].UnmarshalTo(&vmclass)
	if err != nil {
		return nil, err
	}
	return &vmclass, nil
}

// vmClassGenericManifest returns a manifest for 'generic' vmclass
// that should work for VM on every Node in cluster.
func vmClassGenericManifest() *v1alpha2.VirtualMachineClass {
	return &v1alpha2.VirtualMachineClass{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1alpha2.SchemeGroupVersion.String(),
			Kind:       v1alpha2.VirtualMachineClassKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: vmClassGenericName,
			Labels: map[string]string{
				"app":    "virtualization-controller",
				"module": settings.ModuleName,
			},
		},
		Spec: v1alpha2.VirtualMachineClassSpec{
			CPU: v1alpha2.CPU{
				Type:  v1alpha2.CPUTypeModel,
				Model: "Nehalem",
			},
			SizingPolicies: []v1alpha2.SizingPolicy{
				{
					Cores: &v1alpha2.SizingPolicyCores{
						Min: 1,
						Max: 4,
					},
					DedicatedCores: []bool{false},
					CoreFractions:  []v1alpha2.CoreFractionValue{5, 10, 20, 50, 100},
				},
				{
					Cores: &v1alpha2.SizingPolicyCores{
						Min: 5,
						Max: 8,
					},
					DedicatedCores: []bool{false},
					CoreFractions:  []v1alpha2.CoreFractionValue{20, 50, 100},
				},
				{
					Cores: &v1alpha2.SizingPolicyCores{
						Min: 9,
						Max: 16,
					},
					DedicatedCores: []bool{true, false},
					CoreFractions:  []v1alpha2.CoreFractionValue{50, 100},
				},
				{
					Cores: &v1alpha2.SizingPolicyCores{
						Min: 17,
						Max: 1024,
					},
					DedicatedCores: []bool{true, false},
					CoreFractions:  []v1alpha2.CoreFractionValue{100},
				},
			},
		},
	}
}

// isManagedByModule checks if vmclass has all labels that module set when installing vmclass.
func isManagedByModule(vmClass *v1alpha2.VirtualMachineClass) bool {
	if vmClass == nil {
		return false
	}

	expectLabels := vmClassGenericManifest().Labels

	for label, expectValue := range expectLabels {
		actualValue, exists := vmClass.Labels[label]
		if !exists || actualValue != expectValue {
			return false
		}
	}
	return true
}

const (
	heritageLabel            = "heritage"
	helmManagedByLabel       = "app.kubernetes.io/managed-by"
	helmReleaseNameAnno      = "meta.helm.sh/release-name"
	helmReleaseNamespaceAnno = "meta.helm.sh/release-namespace"
	helmKeepResourceAnno     = "helm.sh/resource-policy"
)

// addPatchesToCleanupMetadata fills patch collector with patches if vmclass metadata
// should be cleaned.
func addPatchesToCleanupMetadata(input *pkg.HookInput, vmClass *v1alpha2.VirtualMachineClass) {
	var patches []map[string]interface{}

	labelNames := []string{
		heritageLabel,
		helmManagedByLabel,
	}
	for _, labelName := range labelNames {
		if _, exists := vmClass.Labels[labelName]; exists {
			patches = append(patches, map[string]interface{}{
				"op":    "remove",
				"path":  fmt.Sprintf("/metadata/labels/%s", jsonPatchEscape(labelName)),
				"value": nil,
			})
		}
	}

	// Ensure "keep resource" annotation on vmclass/generic, so Helm will keep it
	// in the cluster even that we've deleted its manifest from templates.
	if _, exists := vmClass.Annotations[helmKeepResourceAnno]; !exists {
		patches = append(patches, map[string]interface{}{
			"op":    "add",
			"path":  fmt.Sprintf("/metadata/annotations/%s", jsonPatchEscape(helmKeepResourceAnno)),
			"value": nil,
		})
	}

	annoNames := []string{
		helmReleaseNameAnno,
		helmReleaseNamespaceAnno,
	}
	for _, annoName := range annoNames {
		if _, exists := vmClass.Annotations[annoName]; exists {
			patches = append(patches, map[string]interface{}{
				"op":    "remove",
				"path":  fmt.Sprintf("/metadata/annotations/%s", jsonPatchEscape(annoName)),
				"value": nil,
			})
		}
	}

	if len(patches) == 0 {
		return
	}

	input.Logger.Info("Patch VirtualMachineClass/generic: remove Helm labels and annotations")
	input.PatchCollector.PatchWithJSON(
		patches,
		vmClass.APIVersion,
		vmClass.Kind,
		vmClass.Namespace,
		vmClass.Name,
	)
}

func jsonPatchEscape(s string) string {
	return strings.NewReplacer("~", "~0", "/", "~1").Replace(s)
}
