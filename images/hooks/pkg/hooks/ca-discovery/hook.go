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

package ca_discovery

import (
	"context"
	"fmt"
	"hooks/pkg/settings"

	"k8s.io/utils/ptr"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/registry"
)

const (
	CommonCASecretSnapshotName = "virtualization-ca"
	CommonCASecretJQFilter     = `{
		"crt": .data."tls.crt",
		"key": .data."tls.key",
	}`

	rootCASecretName = "virtualization-ca"
	rootCAValuesPath = "virtualization.internal.rootCA"
)

type CASecret struct {
	Crt []byte `json:"crt"`
	Key []byte `json:"key"`
}

var _ = registry.RegisterFunc(configModuleCommonCA, handlerModuleCommonCA)

var configModuleCommonCA = &pkg.HookConfig{
	OnBeforeHelm: &pkg.OrderedConfig{Order: 1},
	Kubernetes: []pkg.KubernetesConfig{
		{
			Name:       CommonCASecretSnapshotName,
			APIVersion: "v1",
			Kind:       "Secret",
			JqFilter:   CommonCASecretJQFilter,

			ExecuteHookOnSynchronization: ptr.To(false),

			NameSelector: &pkg.NameSelector{
				MatchNames: []string{rootCASecretName},
			},

			NamespaceSelector: &pkg.NamespaceSelector{
				NameSelector: &pkg.NameSelector{
					MatchNames: []string{settings.ModuleNamespace},
				},
			},
		},
	},

	Queue: fmt.Sprintf("modules/%s", settings.ModuleName),
}

func handlerModuleCommonCA(_ context.Context, input *pkg.HookInput) error {
	caSecret := input.Snapshots.Get(CommonCASecretSnapshotName)

	var rootCA CASecret

	if len(caSecret) == 0 {
		input.Logger.Info(fmt.Sprintf("[ModuleCommonCA] No module's common CA certificate (in secret %s) found. Nothing to do here, another hook will generate CA pair.", settings.ModuleName))

		return nil
	}

	// CA secret is found, decode it and save to Values.
	err := caSecret[0].UnmarshalTo(&rootCA)
	if err != nil {
		return fmt.Errorf("unmarshalTo: %w", err)
	}

	input.Values.Set(rootCAValuesPath, rootCA)

	return nil
}
