/*
Copyright 2026 Flant JSC

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

package moduleconfig

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	mcapi "github.com/deckhouse/virtualization-controller/pkg/controller/moduleconfig/api"
)

const (
	liveMigrationField   = "liveMigration"
	systemNetworkNameKey = "systemNetworkName"
	conditionTypeReady   = "Ready"
	conditionStatusTrue  = "True"
)

var systemNetworkGVK = schema.GroupVersionKind{
	Group:   "network.deckhouse.io",
	Version: "v1alpha1",
	Kind:    "SystemNetwork",
}

type liveMigrationValidator struct {
	client client.Client
}

func newLiveMigrationValidator(c client.Client) *liveMigrationValidator {
	return &liveMigrationValidator{client: c}
}

func (v liveMigrationValidator) ValidateCreate(ctx context.Context, mc *mcapi.ModuleConfig) (admission.Warnings, error) {
	return v.validate(ctx, mc)
}

func (v liveMigrationValidator) ValidateUpdate(ctx context.Context, _, newMC *mcapi.ModuleConfig) (admission.Warnings, error) {
	return v.validate(ctx, newMC)
}

func (v liveMigrationValidator) validate(ctx context.Context, mc *mcapi.ModuleConfig) (admission.Warnings, error) {
	name := parseLiveMigrationSystemNetworkName(mc.Spec.Settings)
	if name == "" {
		return nil, nil
	}

	sn := &unstructured.Unstructured{}
	sn.SetGroupVersionKind(systemNetworkGVK)
	err := v.client.Get(ctx, client.ObjectKey{Name: name}, sn)
	switch {
	case meta.IsNoMatchError(err):
		return nil, fmt.Errorf("liveMigration.systemNetworkName=%q: SDN module is not enabled", name)
	case apierrors.IsNotFound(err):
		return nil, fmt.Errorf("liveMigration.systemNetworkName=%q: SystemNetwork not found", name)
	case err != nil:
		return nil, fmt.Errorf("liveMigration.systemNetworkName=%q: %w", name, err)
	}

	if !isSystemNetworkReady(sn) {
		return nil, fmt.Errorf("liveMigration.systemNetworkName=%q: SystemNetwork is not Ready", name)
	}
	return nil, nil
}

func parseLiveMigrationSystemNetworkName(settings mcapi.SettingsValues) string {
	lm, ok := settings[liveMigrationField].(map[string]any)
	if !ok {
		return ""
	}
	name, _ := lm[systemNetworkNameKey].(string)
	return name
}

func isSystemNetworkReady(sn *unstructured.Unstructured) bool {
	conds, found, err := unstructured.NestedSlice(sn.Object, "status", "conditions")
	if err != nil || !found {
		return false
	}
	for _, c := range conds {
		m, ok := c.(map[string]any)
		if !ok {
			continue
		}
		t, _ := m["type"].(string)
		s, _ := m["status"].(string)
		if t == conditionTypeReady && s == conditionStatusTrue {
			return true
		}
	}
	return false
}
