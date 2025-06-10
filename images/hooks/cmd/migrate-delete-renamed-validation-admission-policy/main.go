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

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"hooks/pkg/common"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/app"
	"github.com/deckhouse/module-sdk/pkg/registry"
	"github.com/deckhouse/module-sdk/pkg/utils/ptr"
)

var _ = registry.RegisterFunc(config, reconcile)

const (
	POLICY_SNAPSHOT_NAME   = "validating_admission_policy"
	BINDING_SNAPSHOT_NAME  = "validating_admission_policy_binding"
	managed_by_label       = "app.kubernetes.io/managed-by"
	managed_by_label_value = "virt-operator-internal-virtualization"
)

var config = &pkg.HookConfig{
	Kubernetes: []pkg.KubernetesConfig{
		{
			Name:       POLICY_SNAPSHOT_NAME,
			APIVersion: "admissionregistration.k8s.io/v1beta1",
			Kind:       "ValidatingAdmissionPolicy",
			NameSelector: &pkg.NameSelector{
				MatchNames: []string{"kubevirt-node-restriction-policy"},
			},
			// JqFilter:                     "{\"name\": .metadata.name, \"kind\": .kind, \"labels\": .metadata.labels}",
			JqFilter:                     ".metadata",
			ExecuteHookOnSynchronization: ptr.Bool(false),
			ExecuteHookOnEvents:          ptr.Bool(false),
		},
		{
			Name:       BINDING_SNAPSHOT_NAME,
			APIVersion: "admissionregistration.k8s.io/v1beta1",
			Kind:       "ValidatingAdmissionPolicyBinding",
			NameSelector: &pkg.NameSelector{
				MatchNames: []string{"kubevirt-node-restriction-binding"},
			},
			// JqFilter:                     "{\"name\": .metadata.name, \"kind\": .kind, \"labels\": .metadata.labels}",
			JqFilter:                     ".metadata",
			ExecuteHookOnSynchronization: ptr.Bool(false),
			ExecuteHookOnEvents:          ptr.Bool(false),
		},
	},
	OnAfterHelm: &pkg.OrderedConfig{Order: 5},
	Queue:       fmt.Sprintf("modules/%s", common.MODULE_NAME),
}

func reconcile(ctx context.Context, input *pkg.HookInput) error {
	input.Logger.Info("Start MigrateDeleteRenamedValidationAadmissionPolicy hook")

	var (
		foundDeprecated int
		uts             []*unstructured.Unstructured
	)

	policySnapshots := input.Snapshots.Get(POLICY_SNAPSHOT_NAME)
	bindingSnapshots := input.Snapshots.Get(BINDING_SNAPSHOT_NAME)

	snapObjs, err := snapsToUnstructured(policySnapshots)
	if err != nil {
		input.Logger.Error("error unmarshalling snapshots for ValidatingAdmissionPolicy")
		return err
	}
	uts = append(uts, snapObjs...)

	snapObjs, err = snapsToUnstructured(bindingSnapshots)
	if err != nil {
		input.Logger.Error("error unmarshalling snapshots for ValidatingAdmissionPolicyBinding")
		return err
	}
	uts = append(uts, snapObjs...)

	c, err := input.DC.GetK8sClient()
	if err != nil {
		input.Logger.Error("Error get kubernetes client %v", err)
		return err
	}

	for _, obj := range uts {
		if obj.GetLabels()[managed_by_label] == managed_by_label_value {
			foundDeprecated++
			name := obj.GetName()
			kind := obj.GetObjectKind().GroupVersionKind().Kind
			input.Logger.Info("Delete deprecated %s %s", name, kind)

			// input.PatchCollector.Delete(apiVersion string, kind string, namespace string, name string)
			err = c.Delete(ctx, obj)
			if err != nil {
				input.Logger.Error("%v, can't delete %s %s", err, name, kind)
			}
		}
	}

	if foundDeprecated == 0 {
		input.Logger.Info("No deprecated resources found, migration not required.")
	}

	return nil
}

func snapsToUnstructured(snaps []pkg.Snapshot) ([]*unstructured.Unstructured, error) {
	objs := make([]*unstructured.Unstructured, 0, len(snaps))

	for i, snap := range snaps {
		ut := &unstructured.Unstructured{}
		err := snap.UnmarshalTo(ut)
		if err != nil {
			return nil, err
		}
		objs[i] = ut
	}

	return objs, nil
}

func main() {
	app.Run()
}
