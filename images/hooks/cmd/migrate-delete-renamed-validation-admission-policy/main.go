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

	"github.com/deckhouse/module-sdk/pkg/app"
	"github.com/deckhouse/module-sdk/pkg/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/registry"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
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

	var clientsObj []client.Object

	policy_snapshots := input.Snapshots.Get(POLICY_SNAPSHOT_NAME)
	binding_snapshots := input.Snapshots.Get(BINDING_SNAPSHOT_NAME)

	for _, binding := range policy_snapshots {
		var ap admissionregistrationv1.ValidatingAdmissionPolicy

		err := binding.UnmarshalTo(&ap)
		if err != nil {
			input.Logger.Error("error unmarshalling snapshot %s", binding.String())
			return err
		}

		clientsObj = append(clientsObj, &ap)
	}

	for _, binding := range binding_snapshots {
		var ap admissionregistrationv1.ValidatingAdmissionPolicyBinding

		err := binding.UnmarshalTo(&ap)
		if err != nil {
			input.Logger.Error("error unmarshalling snapshot %s", binding.String())
			return err
		}

		clientsObj = append(clientsObj, &ap)
	}

	for _, obj := range clientsObj {
		if obj.GetLabels()[managed_by_label] == managed_by_label_value {
			found_deprecated++
			name := obj.GetName()
			kind := obj.GetObjectKind().GroupVersionKind().Kind
			input.Logger.Info("Delete deprecated %s %s", name, kind)

			client, err := input.DC.GetK8sClient()
			if err != nil {
				input.Logger.Error("error unmarshalling snapshot %s", err)
				return err
			}

			err = client.Delete(ctx, obj)
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

func main() {
	app.Run()
}
