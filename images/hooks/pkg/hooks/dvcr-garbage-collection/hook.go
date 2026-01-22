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

package dvcr_garbage_collection

import (
	"context"
	"fmt"
	"hooks/pkg/settings"

	"k8s.io/utils/ptr"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/registry"
)

/**
This hook watches over Secret/dvcr-garbage-collection in d8-virtualization namespace.

If secret is present, hook sets value that switches DVCR Deployment in the garbage collection mode.

When the secret is gone, value is unset.
*/

const (
	SecretSnapshotName = "dvcr-garbage-collection-secret"
	SecretJQFilter     = `{
		"metadata": {
			"name": .metadata.name,
			"annotations": .metadata.annotations,
		},
	}`

	secretName                    = "dvcr-garbage-collection"
	dvcrGarbageCollectionModePath = "virtualization.internal.dvcr.garbageCollectionModeEnabled"

	dvcrDeploymentSwitchToGarbageCollectionModeAnno = "virtualization.deckhouse.io/dvcr-deployment-switch-to-garbage-collection-mode"
)

var _ = registry.RegisterFunc(configDVCRGarbageCollection, handlerDVCRGarbageCollection)

var configDVCRGarbageCollection = &pkg.HookConfig{
	OnBeforeHelm: &pkg.OrderedConfig{Order: 5},
	Kubernetes: []pkg.KubernetesConfig{
		{
			Name:       SecretSnapshotName,
			APIVersion: "v1",
			Kind:       "Secret",
			JqFilter:   SecretJQFilter,

			ExecuteHookOnSynchronization: ptr.To(false),

			NameSelector: &pkg.NameSelector{
				MatchNames: []string{secretName},
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

func handlerDVCRGarbageCollection(_ context.Context, input *pkg.HookInput) error {
	secretSnaps := input.Snapshots.Get(SecretSnapshotName)
	secrets, err := parseSecretSnapshot(secretSnaps)
	if err != nil {
		return err
	}

	input.Values.Set(dvcrGarbageCollectionModePath, isGarbageCollectionEnabled(secrets))
	return nil
}

func isGarbageCollectionEnabled(secrets []partialSecret) string {
	if len(secrets) == 0 {
		return "false"
	}
	if _, ok := secrets[0].Metadata.Annotations[dvcrDeploymentSwitchToGarbageCollectionModeAnno]; ok {
		return "true"
	}

	return "false"
}

type partialSecret struct {
	Metadata partialSecretMetadata `json:"metadata"`
}

type partialSecretMetadata struct {
	Name        string            `json:"name"`
	Annotations map[string]string `json:"annotations"`
}

func parseSecretSnapshot(snaps []pkg.Snapshot) ([]partialSecret, error) {
	secrets := make([]partialSecret, 0, len(snaps))

	if len(snaps) == 0 {
		return secrets, nil
	}

	for _, snap := range snaps {
		var secret partialSecret
		err := snap.UnmarshalTo(&secret)
		if err != nil {
			return nil, err
		}
		secrets = append(secrets, secret)
	}

	return secrets, nil
}
