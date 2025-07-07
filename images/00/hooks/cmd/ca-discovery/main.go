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
	"k8s.io/utils/ptr"
)

const (
	CommonCASecretSnapshotName = "virtualization-ca"
	CommonCASecretJQFilter     = `{
		"crt": .data."tls.crt",
		"key": .data."tls.key",
	}`

	caExpiryDurationStr = "87600h" // 10 years

	// certificate encryption algorithm
	keyAlgorithm = "ecdsa"
	keySize      = 256

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
					MatchNames: []string{common.MODULE_NAMESPACE},
				},
			},
		},
	},

	Queue: fmt.Sprintf("modules/%s", common.MODULE_NAME),
}

func handlerModuleCommonCA(_ context.Context, input *pkg.HookInput) error {
	ca_secret := input.Snapshots.Get(CommonCASecretSnapshotName)

	var rootCA CASecret

	if len(ca_secret) == 0 {
		input.Logger.Info(fmt.Sprintf("[ModuleCommonCA] No module's common CA certificate (in secret %s) found. Nothing to do here, next hook should generate CA pair.", common.MODULE_NAME))

		return nil
	}

	// CA secret is found, decode it and save to Values.
	err := ca_secret[0].UnmarshalTo(&rootCA)
	if err != nil {
		return fmt.Errorf("unmarshalTo: %w", err)
	}

	input.Values.Set(rootCAValuesPath, rootCA)

	return nil
}

func main() {
	app.Run()
}
