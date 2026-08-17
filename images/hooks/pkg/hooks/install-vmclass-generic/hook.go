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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/registry"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/hooks/pkg/settings"
)

const (
	moduleStateSecretSnapshot = "module-state-snapshot"
	moduleStateSecretName     = "module-state"

	vmClassGenericName = "generic"

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
	},

	Queue: fmt.Sprintf("modules/%s", settings.ModuleName),
}

// Reconcile manages the state of vmclass/generic resource:
//
// - Install a new one if there is no state in the Secret indicating that the vmclass was installed earlier.
// - Removes helm related annotations and labels from existing vmclass/generic (one time operation).
// - No actions performed if user deletes or replaces vmclass/generic.
func Reconcile(ctx context.Context, input *pkg.HookInput) error {
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

	vmClassGeneric, err := getVMClassGeneric(ctx, input)
	if err != nil {
		// A failed read is not a reason to stop the module startup: Helm should proceed,
		// e.g. to create the Service for the conversion webhook that may be missing.
		// A read error is not an evidence of absence either, so vmclass/generic is neither
		// created nor patched here, and the state is not set: the next run will retry.
		input.Logger.Error("Skip reconciliation of VirtualMachineClass/generic, cannot read it", "error", err)
		return nil
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

// getVMClassGeneric reads vmclass/generic using Kubernetes client. Returns nil if the vmclass is not found.
//
// Note: getting vmclass with a Kubernetes binding is unreliable when CRD has conversion hooks.
// If conversion webhook is not started yet, deckhouse-controller machinery will
// stuck getting context for this hook and ModuleRun task will be in an error state forever.
// Using Kubernetes client directly fixes this problem.
func getVMClassGeneric(ctx context.Context, input *pkg.HookInput) (*v1alpha2.VirtualMachineClass, error) {
	if input.DC == nil {
		return nil, fmt.Errorf("dependency container is nil")
	}

	k8sClient, err := input.DC.GetK8sClient(addVirtualizationScheme())
	if err != nil {
		return nil, fmt.Errorf("get kubernetes client: %w", err)
	}

	vmClass := &v1alpha2.VirtualMachineClass{}
	err = k8sClient.Get(ctx, client.ObjectKey{Name: vmClassGenericName}, vmClass)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get VirtualMachineClass/%s: %w", vmClassGenericName, err)
	}

	return vmClass, nil
}

type virtualizationSchemeOption struct{}

func (virtualizationSchemeOption) Apply(optsApplier pkg.KubernetesOptionApplier) {
	optsApplier.WithSchemeBuilder(v1alpha2.SchemeBuilder)
}

func addVirtualizationScheme() pkg.KubernetesOption {
	return virtualizationSchemeOption{}
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

	annoNames := []string{
		helmReleaseNameAnno,
		helmReleaseNamespaceAnno,
	}
	hasHelmAnnotations := false
	for _, annoName := range annoNames {
		if _, exists := vmClass.Annotations[annoName]; exists {
			patches = append(patches, map[string]interface{}{
				"op":    "remove",
				"path":  fmt.Sprintf("/metadata/annotations/%s", jsonPatchEscape(annoName)),
				"value": nil,
			})
			hasHelmAnnotations = true
		}
	}

	// Ensure "keep resource" annotation on vmclass/generic, so Helm will keep resource
	// in the cluster even that we've deleted its manifest from templates.
	_, hasKeepResourceAnno := vmClass.Annotations[helmKeepResourceAnno]
	if hasHelmAnnotations && !hasKeepResourceAnno {
		patches = append(patches, map[string]interface{}{
			"op":    "add",
			"path":  fmt.Sprintf("/metadata/annotations/%s", jsonPatchEscape(helmKeepResourceAnno)),
			"value": nil,
		})
	}

	if len(patches) == 0 {
		return
	}

	input.Logger.Info("Patch VirtualMachineClass/generic: remove Helm labels and annotations")
	// Patch the storage version explicitly: a typed client leaves TypeMeta empty, and patching
	// a served version other than the storage one would go through the conversion webhook.
	input.PatchCollector.PatchWithJSON(
		patches,
		v1alpha2.SchemeGroupVersion.String(),
		v1alpha2.VirtualMachineClassKind,
		vmClass.Namespace,
		vmClass.Name,
	)
}

func jsonPatchEscape(s string) string {
	return strings.NewReplacer("~", "~0", "/", "~1").Replace(s)
}
