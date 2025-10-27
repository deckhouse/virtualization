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

package inject_crd_conversion_cabundle

import (
	"context"
	"fmt"

	"hooks/pkg/settings"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/registry"
)

const (
	crdSnapshotName = "virtualmachineclasses-crd"
	crdName         = "virtualmachineclasses.virtualization.deckhouse.io"
	crdJQFilter     = `{
		"name": .metadata.name,
		"hasConversion": (.spec.conversion // null | . != null)
	}`
)

type CRDSnapshot struct {
	Name          string `json:"name"`
	HasConversion bool   `json:"hasConversion"`
}

var _ = registry.RegisterFunc(config, reconcile)

var config = &pkg.HookConfig{
	OnAfterHelm: &pkg.OrderedConfig{Order: 10},
	Kubernetes: []pkg.KubernetesConfig{
		{
			Name:       crdSnapshotName,
			APIVersion: "apiextensions.k8s.io/v1",
			Kind:       "CustomResourceDefinition",
			NameSelector: &pkg.NameSelector{
				MatchNames: []string{crdName},
			},
			JqFilter: crdJQFilter,
		},
	},
	Queue: fmt.Sprintf("modules/%s", settings.ModuleName),
}

func reconcile(ctx context.Context, input *pkg.HookInput) error {
	input.Logger.Info("Start inject CRD conversion webhook configuration hook")

	caCert := input.Values.Get("virtualization.internal.rootCA.crt")
	if !caCert.Exists() {
		input.Logger.Info("CA certificate not found in values, skipping conversion webhook injection")
		return nil
	}

	caBundle := caCert.String()
	if caBundle == "" {
		return fmt.Errorf("CA certificate is empty")
	}

	snapshots := input.Snapshots.Get(crdSnapshotName)
	if len(snapshots) == 0 {
		input.Logger.Info("CRD %s not found, skipping conversion webhook injection", crdName)
		return nil
	}

	var crdSnap CRDSnapshot
	if err := snapshots[0].UnmarshalTo(&crdSnap); err != nil {
		return fmt.Errorf("failed to unmarshal CRD snapshot: %w", err)
	}

	if crdSnap.HasConversion {
		input.Logger.Info("CRD %s already has conversion configuration, skipping injection", crdName)
		return nil
	}

	conversionConfig := map[string]interface{}{
		"strategy": "Webhook",
		"webhook": map[string]interface{}{
			"clientConfig": map[string]interface{}{
				"service": map[string]interface{}{
					"name":      "virtualization-controller",
					"namespace": "d8-virtualization",
					"path":      "/convert",
					"port":      443,
				},
				"caBundle": caBundle,
			},
			"conversionReviewVersions": []string{"v1"},
		},
	}

	patch := []interface{}{
		map[string]interface{}{
			"op":    "add",
			"path":  "/spec/conversion",
			"value": conversionConfig,
		},
	}

	input.PatchCollector.JSONPatch(patch, "apiextensions.k8s.io/v1", "CustomResourceDefinition", "", crdName)
	input.Logger.Info("Successfully injected conversion webhook configuration into CRD %s", crdName)

	return nil
}
