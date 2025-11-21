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

package operation

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service/restorer"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmsopcondition"
)

func NewCreateVirtualMachineOperation(client client.Client, eventRecorder eventrecord.EventRecorderLogger) *CreateVirtualMachineOperation {
	return &CreateVirtualMachineOperation{
		client:   client,
		recorder: eventRecorder,
	}
}

type CreateVirtualMachineOperation struct {
	client   client.Client
	recorder eventrecord.EventRecorderLogger
}

func (o CreateVirtualMachineOperation) Execute(ctx context.Context, vmsop *v1alpha2.VirtualMachineSnapshotOperation) (reconcile.Result, error) {
	cb := conditions.NewConditionBuilder(vmsopcondition.TypeCreateVirtualMachineCompleted)
	defer func() { conditions.SetCondition(cb.Generation(vmsop.Generation), &vmsop.Status.Conditions) }()

	cond, exist := conditions.GetCondition(vmsopcondition.TypeCreateVirtualMachineCompleted, vmsop.Status.Conditions)
	if exist {
		if cond.Status == metav1.ConditionUnknown {
			cb.Status(metav1.ConditionFalse).Reason(vmsopcondition.ReasonCreateVirtualMachineOperationInProgress)
		} else {
			cb.Status(cond.Status).Reason(vmsopcondition.ReasonCreateVirtualMachineCompleted(cond.Reason)).Message(cond.Message)
		}
	}

	if vmsop.Spec.CreateVirtualMachine == nil {
		err := fmt.Errorf("clone specification is mandatory to start creating virtual machine")
		cb.Status(metav1.ConditionFalse).Reason(vmsopcondition.ReasonCreateVirtualMachineOperationFailed).Message(service.CapitalizeFirstLetter(err.Error()))
		return reconcile.Result{}, err
	}

	vmsKey := types.NamespacedName{Namespace: vmsop.Namespace, Name: vmsop.Spec.VirtualMachineSnapshotName}
	vms, err := object.FetchObject(ctx, vmsKey, o.client, &v1alpha2.VirtualMachineSnapshot{})
	if err != nil {
		err := fmt.Errorf("failed to fetch the virtual machine snapshot %q: %w", vmsKey.Name, err)
		cb.Status(metav1.ConditionFalse).Reason(vmsopcondition.ReasonCreateVirtualMachineOperationFailed).Message(service.CapitalizeFirstLetter(err.Error()))
		return reconcile.Result{}, err
	}

	if vms == nil {
		err := fmt.Errorf("specified virtual machine snapshot is not found")
		cb.Status(metav1.ConditionFalse).Reason(vmsopcondition.ReasonCreateVirtualMachineOperationFailed).Message(service.CapitalizeFirstLetter(err.Error()))
		return reconcile.Result{}, err
	}

	restorerSecretKey := types.NamespacedName{Namespace: vms.Namespace, Name: vms.Status.VirtualMachineSnapshotSecretName}
	restorerSecret, err := object.FetchObject(ctx, restorerSecretKey, o.client, &corev1.Secret{})
	if err != nil {
		cb.Status(metav1.ConditionFalse).Reason(vmsopcondition.ReasonCreateVirtualMachineOperationFailed).Message(service.CapitalizeFirstLetter(err.Error()))
		return reconcile.Result{}, err
	}

	if restorerSecret == nil {
		err := fmt.Errorf("restorer secret %q is not found", restorerSecretKey.Name)
		cb.Status(metav1.ConditionFalse).Reason(vmsopcondition.ReasonCreateVirtualMachineOperationFailed).Message(service.CapitalizeFirstLetter(err.Error()))
		return reconcile.Result{}, err
	}

	c, _ := conditions.GetCondition(cb.GetType(), vmsop.Status.Conditions)
	if c.Status == metav1.ConditionTrue {
		return reconcile.Result{}, nil
	}

	snapshotResources := restorer.NewSnapshotResources(o.client, v1alpha2.VMOPTypeClone, vmsop.Spec.CreateVirtualMachine.Mode, restorerSecret, vms, string(vmsop.UID))

	err = snapshotResources.Prepare(ctx)
	if err != nil {
		cb.Status(metav1.ConditionFalse).Reason(vmsopcondition.ReasonCreateVirtualMachineOperationFailed).Message(service.CapitalizeFirstLetter(err.Error()))
		return reconcile.Result{}, err
	}

	snapshotResources.Override(vmsop.Spec.CreateVirtualMachine.NameReplacement)

	if vmsop.Spec.CreateVirtualMachine.Customization != nil {
		snapshotResources.Customize(
			vmsop.Spec.CreateVirtualMachine.Customization.NamePrefix,
			vmsop.Spec.CreateVirtualMachine.Customization.NameSuffix,
		)
	}

	statuses, err := snapshotResources.Validate(ctx)
	vmsop.Status.Resources = statuses
	if err != nil {
		cb.Status(metav1.ConditionFalse).Reason(vmsopcondition.ReasonCreateVirtualMachineOperationFailed).Message(service.CapitalizeFirstLetter(err.Error()))
		return reconcile.Result{}, nil
	}

	if vmsop.Spec.CreateVirtualMachine.Mode == v1alpha2.SnapshotOperationModeDryRun {
		cb.Status(metav1.ConditionTrue).Reason(vmsopcondition.ReasonCreateVirtualMachineOperationCompleted).Message("The virtual machine can be cloned from the snapshot.")
		return reconcile.Result{}, nil
	}

	statuses, err = snapshotResources.Process(ctx)
	vmsop.Status.Resources = statuses
	if err != nil {
		cb.Status(metav1.ConditionFalse).Reason(vmsopcondition.ReasonCreateVirtualMachineOperationFailed).Message(service.CapitalizeFirstLetter(err.Error()))
		return reconcile.Result{}, err
	}

	cb.Status(metav1.ConditionTrue).Reason(vmsopcondition.ReasonCreateVirtualMachineOperationCompleted).Message("The virtual machine has been cloned successfully.")

	return reconcile.Result{}, nil
}

func (o CreateVirtualMachineOperation) IsInProgress(vmsop *v1alpha2.VirtualMachineSnapshotOperation) bool {
	cloneCondition, found := conditions.GetCondition(vmsopcondition.TypeCreateVirtualMachineCompleted, vmsop.Status.Conditions)
	if found && cloneCondition.Status != metav1.ConditionUnknown {
		return true
	}

	return false
}

func (o CreateVirtualMachineOperation) IsComplete(vmsop *v1alpha2.VirtualMachineSnapshotOperation) (bool, string) {
	createVMCondition, ok := conditions.GetCondition(vmsopcondition.TypeCreateVirtualMachineCompleted, vmsop.Status.Conditions)
	if !ok {
		return false, ""
	}

	if createVMCondition.Reason == string(vmsopcondition.ReasonCreateVirtualMachineOperationFailed) {
		return true, createVMCondition.Message
	}

	return createVMCondition.Status == metav1.ConditionTrue, ""
}
