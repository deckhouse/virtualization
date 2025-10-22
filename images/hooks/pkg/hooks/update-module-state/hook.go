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
	genericVMClassStateKey = "generic-vmclass-created"
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
			JqFilter:   `.data`,
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
			genericVMClassStateKey: value,
		},
	}
}

func Reconcile(_ context.Context, input *pkg.HookInput) error {
	vmClasses := input.Snapshots.Get(vmClassSnapshot)
	moduleStateSecrets := input.Snapshots.Get(moduleStateSecretSnapshot)

	vmClassExists := len(vmClasses) > 0

	needsUpdate := false
	hasBeenCreated := false

	if len(moduleStateSecrets) > 0 {
		moduleStateData := make(map[string]interface{})
		if err := moduleStateSecrets[0].UnmarshalTo(&moduleStateData); err == nil {
			if genericCreatedEncoded, exists := moduleStateData[genericVMClassStateKey]; exists {
				if encodedStr, ok := genericCreatedEncoded.(string); ok {
					if decodedBytes, err := base64.StdEncoding.DecodeString(encodedStr); err == nil {
						hasBeenCreated = string(decodedBytes) == "true"
					}
				}
			}
		}
	}

	// Записываем в секрет только когда VMClass создан и еще не записано
	if vmClassExists && !hasBeenCreated {
		needsUpdate = true
		input.Logger.Info("Generic VirtualMachineClass exists but module-state doesn't reflect it was created, updating secret")
	} else if vmClassExists && hasBeenCreated {
		input.Logger.Info("Module-state correctly reflects that generic VirtualMachineClass was created")
	} else if !vmClassExists && hasBeenCreated {
		input.Logger.Info("Generic VirtualMachineClass was created previously but doesn't exist now - user may have deleted it intentionally, keeping historical record")
	} else if !vmClassExists && !hasBeenCreated {
		input.Logger.Info("Generic VirtualMachineClass doesn't exist and was never created - no action needed")
	}

	if needsUpdate {
		state := ModuleState{GenericVMClassCreated: true} // Всегда записываем true, когда VMClass существует

		if len(moduleStateSecrets) > 0 {
			input.PatchCollector.PatchWithMerge(state.ToPatchData(), "v1", "Secret", settings.ModuleNamespace, moduleStateSecretName)
			input.Logger.Info("Updated module-state secret to record that generic VirtualMachineClass was created")
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
				Data: state.ToSecretData(),
				Type: "Opaque",
			}
			input.PatchCollector.Create(secret)
			input.Logger.Info("Created module-state secret to record that generic VirtualMachineClass was created")
		}
	}

	return nil
}
