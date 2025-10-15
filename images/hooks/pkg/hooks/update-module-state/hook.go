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

package update_module_state

import (
	"context"
	"encoding/base64"
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
	updateModuleStateHookName = "Update module-state secret"
	vmClassSnapshot           = "vmclass-generic"
	moduleStateSecretSnapshot = "module-state-secret"

	genericVMClassName    = "generic"
	moduleStateSecretName = "module-state"

	apiVersion = core.GroupName + "/" + v1alpha2.Version

	// State fields configuration
	genericVMClassStateKey = "generic-vmclass-was-ever-created"
)

var _ = registry.RegisterFunc(config, Reconcile)

var config = &pkg.HookConfig{
	OnBeforeHelm: &pkg.OrderedConfig{Order: 15},
	Kubernetes: []pkg.KubernetesConfig{
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
	},

	Queue: fmt.Sprintf("modules/%s", settings.ModuleName),
}

type ModuleState struct {
	GenericVMClassCreated bool
}

func (ms ModuleState) ToSecretData() map[string][]byte {
	value := fmt.Sprintf("%t", ms.GenericVMClassCreated)
	return map[string][]byte{
		genericVMClassStateKey: []byte(value),
	}
}

func (ms ModuleState) ToPatchData() map[string]interface{} {
	value := fmt.Sprintf("%t", ms.GenericVMClassCreated)
	return map[string]interface{}{
		"data": map[string]string{
			genericVMClassStateKey: base64.StdEncoding.EncodeToString([]byte(value)),
		},
	}
}

func Reconcile(_ context.Context, input *pkg.HookInput) error {
	vmClasses := input.Snapshots.Get(vmClassSnapshot)
	moduleStateSecrets := input.Snapshots.Get(moduleStateSecretSnapshot)

	vmClassExists := len(vmClasses) > 0

	// Load existing state
	currentState := ModuleState{GenericVMClassCreated: false}
	if len(moduleStateSecrets) > 0 {
		var moduleStateSecret corev1.Secret
		if err := moduleStateSecrets[0].UnmarshalTo(&moduleStateSecret); err == nil {
			if string(moduleStateSecret.Data[genericVMClassStateKey]) == "true" {
				currentState.GenericVMClassCreated = true
			}
		}
	}

	// Update state: generic-vmclass-was-ever-created can only transition from false to true
	newState := ModuleState{
		GenericVMClassCreated: currentState.GenericVMClassCreated || vmClassExists,
	}

	// Always ensure secret exists with current state
	if len(moduleStateSecrets) > 0 {
		input.PatchCollector.PatchWithMerge(newState.ToPatchData(), "v1", "Secret", settings.ModuleNamespace, moduleStateSecretName)
	} else {
		secret := &corev1.Secret{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1",
				Kind:       "Secret",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      moduleStateSecretName,
				Namespace: settings.ModuleNamespace,
				Labels: map[string]string{
					"module": settings.ModuleName,
				},
			},
			Data: newState.ToSecretData(),
			Type: "Opaque",
		}
		input.PatchCollector.Create(secret)
	}

	return nil
}
