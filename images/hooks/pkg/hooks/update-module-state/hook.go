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

func Reconcile(_ context.Context, input *pkg.HookInput) error {
	vmClasses := input.Snapshots.Get(vmClassSnapshot)
	moduleStateSecrets := input.Snapshots.Get(moduleStateSecretSnapshot)

	vmClassExists := len(vmClasses) > 0

	needsUpdate := false
	currentState := false

	if len(moduleStateSecrets) > 0 {
		moduleStateData := make(map[string]interface{})
		if err := moduleStateSecrets[0].UnmarshalTo(&moduleStateData); err == nil {
			if genericCreatedEncoded, exists := moduleStateData["generic-vmclass-created"]; exists {
				if encodedStr, ok := genericCreatedEncoded.(string); ok {
					if decodedBytes, err := base64.StdEncoding.DecodeString(encodedStr); err == nil {
						currentState = string(decodedBytes) == "true"
					}
				}
			}
		}
	}

	if vmClassExists && !currentState {
		needsUpdate = true
		input.Logger.Info("Generic VirtualMachineClass exists but module-state doesn't reflect this, updating secret")
	} else if !vmClassExists && currentState {
		needsUpdate = true
		input.Logger.Info("Generic VirtualMachineClass doesn't exist but module-state indicates it was created, updating secret")
	} else if len(moduleStateSecrets) == 0 {
		needsUpdate = true
		input.Logger.Info("Module-state secret doesn't exist, creating it")
	} else if vmClassExists && currentState {
		input.Logger.Info("Module-state correctly reflects that generic vmclass exists")
	} else {
		input.Logger.Info("Module-state correctly reflects that generic vmclass doesn't exist")
	}

	if needsUpdate {
		if len(moduleStateSecrets) > 0 {
			patchData := map[string]interface{}{
				"data": map[string]string{
					"generic-vmclass-created": base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%t", vmClassExists))),
				},
			}
			input.PatchCollector.PatchWithMerge(patchData, "v1", "Secret", settings.ModuleNamespace, moduleStateSecretName)
			input.Logger.Info("Updated module-state secret")
		} else {
			secretData := map[string]string{
				"generic-vmclass-created": fmt.Sprintf("%t", vmClassExists),
			}

			encodedData := make(map[string][]byte)
			for key, value := range secretData {
				encodedData[key] = []byte(base64.StdEncoding.EncodeToString([]byte(value)))
			}

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
				Data: encodedData,
				Type: "Opaque",
			}
			input.PatchCollector.Create(secret)
			input.Logger.Info("Created module-state secret")
		}
	}

	return nil
}
