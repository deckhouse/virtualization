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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmscondition"
)

type CreateSnapshotStep struct {
	client   client.Client
	recorder eventrecord.EventRecorderLogger
}

func NewCreateSnapshotStep(client client.Client, recorder eventrecord.EventRecorderLogger) *CreateSnapshotStep {
	return &CreateSnapshotStep{
		client:   client,
		recorder: recorder,
	}
}

func (s CreateSnapshotStep) Take(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (*reconcile.Result, error) {
	rcb := conditions.NewConditionBuilder(vmopcondition.TypeSnapshotReady)

	if snapshotCondition, found := conditions.GetCondition(vmopcondition.TypeSnapshotReady, vmop.Status.Conditions); found {
		if snapshotCondition.Status == metav1.ConditionTrue || snapshotCondition.Reason == string(vmopcondition.ReasonSnapshotCleanedUp) {
			return nil, nil
		}
	}

	if snapshotName, exists := vmop.Annotations[annotations.AnnVMOPSnapshotName]; exists {
		vmSnapshotKey := types.NamespacedName{Namespace: vmop.Namespace, Name: snapshotName}
		vmSnapshot, err := object.FetchObject(ctx, vmSnapshotKey, s.client, &v1alpha2.VirtualMachineSnapshot{})
		if err != nil {
			return &reconcile.Result{}, err
		}

		if vmSnapshot != nil {
			switch vmSnapshot.Status.Phase {
			case v1alpha2.VirtualMachineSnapshotPhaseFailed:
				conditions.SetCondition(
					rcb.Status(metav1.ConditionFalse).Reason(vmopcondition.ReasonSnapshotFailed).Message("Snapshot is failed."),
					&vmop.Status.Conditions,
				)
				vmsReadyCondition, _ := conditions.GetCondition(vmscondition.VirtualMachineSnapshotReadyType, vmSnapshot.Status.Conditions)
				err = fmt.Errorf("virtual machine %q have invalid state: %s", vmop.Spec.VirtualMachine, vmsReadyCondition.Message)
				return &reconcile.Result{}, fmt.Errorf("virtual machine snapshot %q is in failed phase: %w. Try again with new VMOP Clone operation", vmSnapshotKey.Name, err)
			case v1alpha2.VirtualMachineSnapshotPhaseReady:
				conditions.SetCondition(
					rcb.Status(metav1.ConditionTrue).Reason(vmopcondition.ReasonSnapshotOperationReady).Message("Snapshot is ready for clone operation."),
					&vmop.Status.Conditions,
				)
				return nil, nil
			}
		}

		conditions.SetCondition(
			rcb.Status(metav1.ConditionFalse).Reason(vmopcondition.ReasonSnapshotInProgress).Message("Snapshot creation is in progress."),
			&vmop.Status.Conditions,
		)
		return &reconcile.Result{}, nil
	}

	var snapshotList v1alpha2.VirtualMachineSnapshotList
	err := s.client.List(ctx, &snapshotList, client.InNamespace(vmop.Namespace))
	if err != nil {
		return &reconcile.Result{}, err
	}

	for _, snapshot := range snapshotList.Items {
		for _, owner := range snapshot.OwnerReferences {
			if owner.UID == vmop.UID {
				if vmop.Spec.Clone != nil {
					if vmop.Annotations == nil {
						vmop.Annotations = make(map[string]string)
					}
					vmop.Annotations[annotations.AnnVMOPSnapshotName] = snapshot.Name
				}
				return &reconcile.Result{}, nil
			}
		}
	}

	snapshot := &v1alpha2.VirtualMachineSnapshot{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1alpha2.VirtualMachineSnapshotKind,
			APIVersion: v1alpha2.Version,
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "vmop-clone-",
			Namespace:    vmop.Namespace,
			Annotations: map[string]string{
				annotations.AnnVMOPUID: string(vmop.UID),
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         v1alpha2.SchemeGroupVersion.String(),
					Kind:               v1alpha2.VirtualMachineOperationKind,
					Name:               vmop.Name,
					UID:                vmop.UID,
					BlockOwnerDeletion: ptr.To(true),
				},
			},
		},
		Spec: v1alpha2.VirtualMachineSnapshotSpec{
			VirtualMachineName:  vmop.Spec.VirtualMachine,
			KeepIPAddress:       v1alpha2.KeepIPAddressNever,
			RequiredConsistency: true,
		},
	}

	err = s.client.Create(ctx, snapshot)
	if err != nil {
		return &reconcile.Result{}, fmt.Errorf("failed to create VirtualMachineSnapshot: %w", err)
	}

	if vmop.Annotations == nil {
		vmop.Annotations = make(map[string]string)
	}
	vmop.Annotations[annotations.AnnVMOPSnapshotName] = snapshot.Name

	s.recorder.Event(vmop, corev1.EventTypeNormal, "SnapshotCreated", fmt.Sprintf("Created snapshot %s for clone operation", snapshot.Name))

	conditions.SetCondition(
		rcb.Status(metav1.ConditionFalse).Reason(vmopcondition.ReasonSnapshotInProgress).Message("Snapshot creation is in progress."),
		&vmop.Status.Conditions,
	)

	return &reconcile.Result{}, nil
}
