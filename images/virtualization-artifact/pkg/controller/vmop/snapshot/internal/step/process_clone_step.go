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

package step

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service/restorer"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type ProcessCloneStep struct {
	client   client.Client
	recorder eventrecord.EventRecorderLogger
}

func NewProcessCloneStep(
	client client.Client,
	recorder eventrecord.EventRecorderLogger,
) *ProcessCloneStep {
	return &ProcessCloneStep{
		client:   client,
		recorder: recorder,
	}
}

func (s ProcessCloneStep) Take(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (*reconcile.Result, error) {
	snapshotName, ok := vmop.Annotations[annotations.AnnVMOPSnapshotName]
	if !ok {
		err := fmt.Errorf("snapshot name annotation not found")
		return &reconcile.Result{}, err
	}

	vmSnapshotKey := types.NamespacedName{Namespace: vmop.Namespace, Name: snapshotName}
	vmSnapshot, err := object.FetchObject(ctx, vmSnapshotKey, s.client, &v1alpha2.VirtualMachineSnapshot{})
	if err != nil {
		return &reconcile.Result{}, err
	}

	if vmSnapshot == nil {
		return &reconcile.Result{}, nil
	}

	restorerSecretKey := types.NamespacedName{Namespace: vmSnapshot.Namespace, Name: vmSnapshot.Status.VirtualMachineSnapshotSecretName}
	restorerSecret, err := object.FetchObject(ctx, restorerSecretKey, s.client, &corev1.Secret{})
	if err != nil {
		return &reconcile.Result{}, err
	}

	if restorerSecret == nil {
		return &reconcile.Result{}, nil
	}

	snapshotResources := restorer.NewSnapshotResources(s.client, v1alpha2.VMOPTypeClone, vmop.Spec.Clone.Mode, restorerSecret, vmSnapshot, string(vmop.UID))

	err = snapshotResources.Prepare(ctx)
	if err != nil {
		return &reconcile.Result{}, err
	}

	snapshotResources.Override(vmop.Spec.Clone.NameReplacement)

	if vmop.Spec.Clone.Customization != nil {
		snapshotResources.Customize(
			vmop.Spec.Clone.Customization.NamePrefix,
			vmop.Spec.Clone.Customization.NameSuffix,
		)
	}

	statuses, err := snapshotResources.Validate(ctx)
	vmop.Status.Resources = statuses
	if err != nil {
		return &reconcile.Result{}, err
	}

	if vmop.Spec.Clone.Mode == v1alpha2.SnapshotOperationModeDryRun {
		return &reconcile.Result{}, nil
	}

	statuses, err = snapshotResources.Process(ctx)
	vmop.Status.Resources = statuses
	if err != nil {
		return &reconcile.Result{}, err
	}

	return nil, nil
}
