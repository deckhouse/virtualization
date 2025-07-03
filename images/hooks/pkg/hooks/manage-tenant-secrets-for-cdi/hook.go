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

package manage_tenant_secrets_for_cdi

import (
	"context"
	"fmt"
	"reflect"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/registry"
	"github.com/deckhouse/module-sdk/pkg/utils/ptr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"hooks/pkg/settings"
)

const (
	podSnapshotName       = "pods"
	secretsSnapshotName   = "secrets"
	namespaceSnapshotName = "namespaces"

	sourceSecretName = "virtualization-module-registry"
)

var destinationSecretLabels = map[string]string{
	"heritage": "deckhouse",
	"kubevirt.deckhouse.io/cdi-registry-secret": "true",
	"deckhouse.io/registry-secret":              "true",
}

var _ = registry.RegisterFunc(config, reconcile)

var config = &pkg.HookConfig{
	Kubernetes: []pkg.KubernetesConfig{
		{
			Name:       podSnapshotName,
			APIVersion: "v1",
			Kind:       "Pod",
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":                          "containerized-data-importer",
					"app.kubernetes.io/managed-by": "cdi-controller-internal-virtualization",
				},
			},
			JqFilter:                     `{"namespace": .metadata.namespace}`,
			ExecuteHookOnSynchronization: ptr.Bool(false),
			ExecuteHookOnEvents:          ptr.Bool(false),
		},
		{
			Name:       secretsSnapshotName,
			APIVersion: "v1",
			Kind:       "Secret",
			NameSelector: &pkg.NameSelector{
				MatchNames: []string{sourceSecretName},
			},
			JqFilter:                     `{"data": .data, "namespace": .metadata.namespace, "type": .type}`,
			ExecuteHookOnSynchronization: ptr.Bool(false),
			ExecuteHookOnEvents:          ptr.Bool(false),
		},
		{
			Name:       namespaceSnapshotName,
			APIVersion: "v1",
			Kind:       "Namespace",
			JqFilter: `{
                "name": .metadata.name,
                "isTerminating": any(.metadata; .deletionTimestamp != null)
            }`,
			ExecuteHookOnSynchronization: ptr.Bool(false),
			ExecuteHookOnEvents:          ptr.Bool(false),
		},
	},
	OnAfterHelm: &pkg.OrderedConfig{Order: 5},
	Queue:       fmt.Sprintf("modules/%s", settings.ModuleName),
}

func reconcile(_ context.Context, input *pkg.HookInput) error {
	input.Logger.Info("Starting ManageTenantSecrets hook")

	sourceNamespace := settings.ModuleNamespace

	podSnapshots := input.Snapshots.Get(podSnapshotName)
	podNamespaces := make(map[string]bool)
	for _, p := range podSnapshots {
		var ns struct {
			Namespace string `json:"namespace"`
		}
		if err := p.UnmarshalTo(&ns); err != nil {
			input.Logger.Error("Failed to unmarshal pod snapshot: %v", err)
			continue
		}
		podNamespaces[ns.Namespace] = true
	}

	nsSnapshots := input.Snapshots.Get(namespaceSnapshotName)
	for _, s := range nsSnapshots {
		var ns struct {
			Name          string `json:"name"`
			IsTerminating bool   `json:"isTerminating"`
		}
		if err := s.UnmarshalTo(&ns); err != nil {
			input.Logger.Error("Failed to unmarshal namespace snapshot: %v", err)
			continue
		}
		if ns.IsTerminating {
			delete(podNamespaces, ns.Name)
		}
	}

	var sourceData map[string]interface{}
	var secretType string
	secretsSnapshots := input.Snapshots.Get(secretsSnapshotName)
	secretsByNs := make(map[string]map[string]interface{})
	for _, s := range secretsSnapshots {
		var sec struct {
			Data      map[string]interface{} `json:"data"`
			Namespace string                 `json:"namespace"`
			Type      string                 `json:"type"`
		}
		if err := s.UnmarshalTo(&sec); err != nil {
			input.Logger.Error("Failed to unmarshal secret snapshot: %v", err)
			continue
		}
		if sec.Namespace == sourceNamespace {
			sourceData = sec.Data
			secretType = sec.Type
			continue
		}
		secretsByNs[sec.Namespace] = sec.Data
	}

	if len(sourceData) == 0 || secretType == "" {
		input.Logger.Warn("Source secret not found: %s/%s", sourceNamespace, sourceSecretName)
		return nil
	}

	for ns := range podNamespaces {
		if ns == sourceNamespace {
			continue
		}
		existingData := secretsByNs[ns]
		if !reflect.DeepEqual(existingData, sourceData) {
			input.Logger.Info("Creating/updating secret: %s/%s", ns, sourceSecretName)
			secret := generateSecret(ns, sourceData, secretType)
			input.PatchCollector.Create(secret)
		}
	}

	for ns := range secretsByNs {
		if _, ok := podNamespaces[ns]; !ok && ns != sourceNamespace {
			input.Logger.Info("Deleting secret: %s/%s", ns, sourceSecretName)
			input.PatchCollector.Delete("v1", "Secret", ns, sourceSecretName)
		}
	}

	return nil
}

func generateSecret(namespace string, data map[string]interface{}, secretType string) *unstructured.Unstructured {
	secret := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      sourceSecretName,
				"namespace": namespace,
				"labels":    destinationSecretLabels,
			},
			"data": data,
			"type": secretType,
		},
	}
	return secret
}
