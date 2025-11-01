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

package create_generic_vmclass

import (
	"context"
	"fmt"

	"hooks/pkg/settings"

	"github.com/deckhouse/virtualization/api/core"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/registry"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

const (
	moduleStateSecretSnapshot = "module-state-secret"
	vmClassSnapshot           = "vmclass-generic"

	moduleStateSecretName = "module-state"
	genericVMClassName    = "generic"

	apiVersion = core.GroupName + "/" + v1alpha2.Version
)

var _ = registry.RegisterFunc(config, Reconcile)

var config = &pkg.HookConfig{
	OnBeforeHelm: &pkg.OrderedConfig{Order: 5},
	Kubernetes: []pkg.KubernetesConfig{
		{
			Name:       moduleStateSecretSnapshot,
			APIVersion: "v1",
			Kind:       "Secret",
			JqFilter:   `{"metadata": .metadata, "data": .data}`,
			NameSelector: &pkg.NameSelector{
				MatchNames: []string{moduleStateSecretName},
			},
			NamespaceSelector: &pkg.NamespaceSelector{
				NameSelector: &pkg.NameSelector{
					MatchNames: []string{settings.ModuleNamespace},
				},
			},
			ExecuteHookOnSynchronization: ptr.To(false),
		},
		{
			Name:       vmClassSnapshot,
			APIVersion: apiVersion,
			Kind:       v1alpha2.VirtualMachineClassKind,
			JqFilter:   `.metadata.name`,
			NameSelector: &pkg.NameSelector{
				MatchNames: []string{genericVMClassName},
			},
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":    "virtualization-controller",
					"module": settings.ModuleName,
				},
			},
			ExecuteHookOnSynchronization: ptr.To(false),
		},
	},

	Queue: fmt.Sprintf("modules/%s", settings.ModuleName),
}

func Reconcile(_ context.Context, input *pkg.HookInput) error {
	moduleStateSecrets := input.Snapshots.Get(moduleStateSecretSnapshot)
	vmClasses := input.Snapshots.Get(vmClassSnapshot)

	// nothing to do if generic vmclass already exists
	if len(vmClasses) > 0 {
		return nil
	}

	// if module-state secret exists and contains generic-vmclass-was-ever-created=true, nothing to do
	if len(moduleStateSecrets) > 0 {
		var moduleStateSecret corev1.Secret
		if err := moduleStateSecrets[0].UnmarshalTo(&moduleStateSecret); err != nil {
			return err
		}

		if string(moduleStateSecret.Data["generic-vmclass-was-ever-created"]) == "true" {
			return nil
		}
	}

	input.Logger.Info("Creating generic VirtualMachineClass")

	vmClass := &v1alpha2.VirtualMachineClass{
		TypeMeta: metav1.TypeMeta{
			APIVersion: apiVersion,
			Kind:       v1alpha2.VirtualMachineClassKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: genericVMClassName,
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

	input.PatchCollector.Create(vmClass)
	input.Logger.Info("VirtualMachineClass generic created")

	return nil
}
