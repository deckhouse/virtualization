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

	"github.com/deckhouse/module-sdk/pkg/utils/ptr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/registry"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	// admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	//"k8s.io/api/admissionregistration/v1"
)

type VAP struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              admissionregistrationv1.ValidatingAdmissionPolicy `json:"spec,omitempty"`
}
type VAPB struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              admissionregistrationv1.ValidatingAdmissionPolicyBinding `json:"spec,omitempty"`
}

// type snapShot struct {
// 	metav1.TypeMeta   `json:",inline"`
// 	metav1.ObjectMeta `json:"metadata,omitempty"`
// 	// filterResult `json:"filterResult"`
// }

// type filterResult struct {
// 	Name   string   `json:"name"`
// 	Labels []string `json:"labels"`
// 	Kind   string   `json:"kind"`
// }

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
			JqFilter:                     "{\"name\": .metadata.name, \"kind\": .kind, \"labels\": .metadata.labels}",
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
			JqFilter:                     "{\"name\": .metadata.name, \"kind\": .kind, \"labels\": .metadata.labels}",
			ExecuteHookOnSynchronization: ptr.Bool(false),
			ExecuteHookOnEvents:          ptr.Bool(false),
		},
	},
	Queue: fmt.Sprintf("modules/%s", common.MODULE_NAME),
}

func reconcile(ctx context.Context, input *pkg.HookInput) error {
	input.Logger.Info("hello from patch hook")
	found_deprecated := 0

	// var snapShots pkg.Snapshots
	var apb VAP
	// var snapShots snapShot

	binding_snapshot := input.Snapshots.Get(POLICY_SNAPSHOT_NAME)
	// policy_snapshot := input.Snapshots.Get(POLICY_SNAPSHOT_NAME)

	for _, binding := range binding_snapshot {

		// metadata := &metav1.ObjectMeta{}

		err := binding.UnmarshalTo(&apb)
		if err != nil {
			input.Logger.Error("error unmarshalling snapshot %s", binding.String())
			return err
		}

		// if _, ok := binding.UnmarshalTo(v any)
		// // if _, ok := snapShots.Labels[managed_by_label]; ok {
		// 	continue
		// }

		if apb.Labels[managed_by_label] == managed_by_label_value {
			found_deprecated++
			name := apb.Name
			kind := apb.Kind
			input.Logger.Info("Delete deprecated %s %s", name, kind)

			client, err := input.DC.GetK8sClient()
			if err != nil {
				input.Logger.Error("error unmarshalling snapshot %s", err)
				return err
			}

			vap := admissionregistrationv1.ValidatingAdmissionPolicy{ObjectMeta: metav1.ObjectMeta{Name: name}}

			err = client.Delete(ctx, &vap)
			if err != nil {
				continue
			}

		}
	}

	if found_deprecated == 0 {
		input.Logger.Info("No deprecated resources found, migration not required.")
	}

	return nil
}
