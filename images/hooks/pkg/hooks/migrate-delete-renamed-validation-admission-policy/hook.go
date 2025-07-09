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

package migrate_delete_renamed_validation_admission_policy

import (
	"context"
	"fmt"

	"hooks/pkg/settings"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/registry"
	"github.com/deckhouse/module-sdk/pkg/utils/ptr"
)

type vap struct {
	Kind       string            `json:"kind"`
	Name       string            `json:"name"`
	ApiVersion string            `json:"apiVersion"`
	Labels     map[string]string `json:"labels"`
}

var _ = registry.RegisterFunc(config, reconcile)

const (
	policySnapshotName  = "validating_admission_policy"
	bindingSnapshotName = "validating_admission_policy_binding"
	managedByLabel      = "app.kubernetes.io/managed-by"
	managedByLabelValue = "virt-operator-internal-virtualization"
)

var config = &pkg.HookConfig{
	Kubernetes: []pkg.KubernetesConfig{
		{
			Name:       policySnapshotName,
			APIVersion: "admissionregistration.k8s.io/v1beta1",
			Kind:       "ValidatingAdmissionPolicy",
			NameSelector: &pkg.NameSelector{
				MatchNames: []string{"kubevirt-node-restriction-policy"},
			},
			JqFilter:                     ".metadata",
			ExecuteHookOnSynchronization: ptr.Bool(false),
			ExecuteHookOnEvents:          ptr.Bool(false),
		},
		{
			Name:       bindingSnapshotName,
			APIVersion: "admissionregistration.k8s.io/v1beta1",
			Kind:       "ValidatingAdmissionPolicyBinding",
			NameSelector: &pkg.NameSelector{
				MatchNames: []string{"kubevirt-node-restriction-binding"},
			},
			JqFilter:                     ".metadata",
			ExecuteHookOnSynchronization: ptr.Bool(false),
			ExecuteHookOnEvents:          ptr.Bool(false),
		},
	},
	OnAfterHelm: &pkg.OrderedConfig{Order: 5},
	Queue:       fmt.Sprintf("modules/%s", settings.ModuleName),
}

func reconcile(ctx context.Context, input *pkg.HookInput) error {
	input.Logger.Info("Start MigrateDeleteRenamedValidationAadmissionPolicy hook")

	var (
		foundDeprecatedCount int
		uts                  []*vap
	)

	policySnapshots := input.Snapshots.Get(policySnapshotName)
	bindingSnapshots := input.Snapshots.Get(bindingSnapshotName)

	snapObjs, err := snapsToUnstructured(policySnapshots)
	if err != nil {
		input.Logger.Error("Error unmarshalling snapshots for ValidatingAdmissionPolicy")
		return err
	}
	uts = append(uts, snapObjs...)

	snapObjs, err = snapsToUnstructured(bindingSnapshots)
	if err != nil {
		input.Logger.Error("Error unmarshalling snapshots for ValidatingAdmissionPolicyBinding")
		return err
	}
	uts = append(uts, snapObjs...)

	for _, obj := range uts {
		if obj.Labels[managedByLabel] == managedByLabelValue {
			foundDeprecatedCount++
			name := obj.Name
			kind := obj.Kind
			apiVersion := obj.ApiVersion
			input.Logger.Info("Delete deprecated %s %s", name, kind)

			input.PatchCollector.Delete(apiVersion, kind, "", name)
		}
	}

	if foundDeprecatedCount == 0 {
		input.Logger.Info("No deprecated resources found, migration not required.")
	}

	return nil
}

func snapsToUnstructured(snaps []pkg.Snapshot) ([]*vap, error) {
	objs := make([]*vap, len(snaps))

	for i, snap := range snaps {
		ut := &vap{}
		if err := snap.UnmarshalTo(ut); err != nil {
			return nil, err
		}
		objs[i] = ut
	}

	return objs, nil
}
