/*
Copyright 2024 Flant JSC

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

package internal

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmrestore/internal/restorer"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	vmrestorecondition "github.com/deckhouse/virtualization/api/core/v1alpha2/vm-restore-condition"
)

type LifeCycleHandler struct {
	client   client.Client
	restorer Restorer
}

func NewLifeCycleHandler(client client.Client, restorer Restorer) *LifeCycleHandler {
	return &LifeCycleHandler{
		client:   client,
		restorer: restorer,
	}
}

func (h LifeCycleHandler) Handle(ctx context.Context, vmRestore *virtv2.VirtualMachineRestore) (reconcile.Result, error) {
	switch vmRestore.Status.Phase {
	case virtv2.VirtualMachineRestorePhaseReady,
		virtv2.VirtualMachineRestorePhaseFailed,
		virtv2.VirtualMachineRestorePhaseTerminating:
		return reconcile.Result{}, nil
	}

	cb := conditions.NewConditionBuilder(vmrestorecondition.VirtualMachineRestoreReadyType)
	defer func() { conditions.SetCondition(cb.Generation(vmRestore.Generation), &vmRestore.Status.Conditions) }()

	if !conditions.HasCondition(cb.GetType(), vmRestore.Status.Conditions) {
		cb.Status(metav1.ConditionUnknown).Reason(conditions.ReasonUnknown)
	}

	if vmRestore.Status.Phase == "" {
		vmRestore.Status.Phase = virtv2.VirtualMachineRestorePhasePending
	}

	if vmRestore.DeletionTimestamp != nil {
		vmRestore.Status.Phase = virtv2.VirtualMachineRestorePhaseTerminating
		cb.Status(metav1.ConditionUnknown).Reason(conditions.ReasonUnknown)
		return reconcile.Result{}, nil
	}

	if vmRestore.Status.Phase == virtv2.VirtualMachineRestorePhaseInProgress {
		vmRestore.Status.Phase = virtv2.VirtualMachineRestorePhaseReady
		cb.Status(metav1.ConditionTrue).Reason(vmrestorecondition.VirtualMachineRestoreReady)
		return reconcile.Result{}, nil
	}

	vmSnapshotReadyToUseCondition, _ := conditions.GetCondition(vmrestorecondition.VirtualMachineSnapshotReadyToUseType, vmRestore.Status.Conditions)
	if vmSnapshotReadyToUseCondition.Status != metav1.ConditionTrue {
		vmRestore.Status.Phase = virtv2.VirtualMachineRestorePhasePending
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmrestorecondition.VirtualMachineSnapshotNotReadyToUse).
			Message(fmt.Sprintf("Waiting for the virtual machine snapshot %q to be ready to use.", vmRestore.Spec.VirtualMachineSnapshotName))
		return reconcile.Result{}, nil
	}

	vmSnapshotKey := types.NamespacedName{Namespace: vmRestore.Namespace, Name: vmRestore.Spec.VirtualMachineSnapshotName}
	vmSnapshot, err := object.FetchObject(ctx, vmSnapshotKey, h.client, &virtv2.VirtualMachineSnapshot{})
	if err != nil {
		setPhaseConditionToFailed(cb, &vmRestore.Status.Phase, err)
		return reconcile.Result{}, err
	}

	if vmSnapshot == nil {
		err = fmt.Errorf("the virtual machine snapshot %q is nil, please report a bug", vmRestore.Spec.VirtualMachineSnapshotName)
		setPhaseConditionToFailed(cb, &vmRestore.Status.Phase, err)
		return reconcile.Result{}, err
	}

	restorerSecretKey := types.NamespacedName{Namespace: vmSnapshot.Namespace, Name: vmSnapshot.Status.VirtualMachineSnapshotSecretName}
	restorerSecret, err := object.FetchObject(ctx, restorerSecretKey, h.client, &corev1.Secret{})
	if err != nil {
		setPhaseConditionToFailed(cb, &vmRestore.Status.Phase, err)
		return reconcile.Result{}, err
	}

	var overrideValidators []OverrideValidator

	vm, err := h.restorer.RestoreVirtualMachine(ctx, restorerSecret)
	if err != nil {
		setPhaseConditionToFailed(cb, &vmRestore.Status.Phase, err)
		return reconcile.Result{}, err
	}

	vmip, err := h.restorer.RestoreVirtualMachineIPAddress(ctx, restorerSecret)
	if err != nil {
		setPhaseConditionToFailed(cb, &vmRestore.Status.Phase, err)
		return reconcile.Result{}, err
	}

	if vmip != nil {
		vm.Spec.VirtualMachineIPAddress = vmip.Name
		overrideValidators = append(overrideValidators, restorer.NewVirtualMachineIPAddressOverrideValidator(vmip, h.client))
	}

	overrideValidators = append(overrideValidators, restorer.NewVirtualMachineOverrideValidator(vm, h.client))

	vds, err := h.getVirtualDisks(ctx, vmSnapshot)
	switch {
	case err == nil:
	case errors.Is(err, ErrVirtualDiskSnapshotNotFound):
		vmRestore.Status.Phase = virtv2.VirtualMachineRestorePhasePending
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmrestorecondition.VirtualMachineSnapshotNotReady).
			Message(service.CapitalizeFirstLetter(err.Error()) + ".")
		return reconcile.Result{}, nil
	default:
		setPhaseConditionToFailed(cb, &vmRestore.Status.Phase, err)
		return reconcile.Result{}, err
	}

	vmbdas, err := h.restorer.RestoreVirtualMachineBlockDeviceAttachments(ctx, restorerSecret)
	if err != nil {
		setPhaseConditionToFailed(cb, &vmRestore.Status.Phase, err)
		return reconcile.Result{}, err
	}

	provisioner, err := h.restorer.RestoreProvisioner(ctx, restorerSecret)
	if err != nil {
		setPhaseConditionToFailed(cb, &vmRestore.Status.Phase, err)
		return reconcile.Result{}, err
	}

	for _, vd := range vds {
		overrideValidators = append(overrideValidators, restorer.NewVirtualDiskOverrideValidator(vd, h.client))
	}

	for _, vmbda := range vmbdas {
		overrideValidators = append(overrideValidators, restorer.NewVirtualMachineBlockDeviceAttachmentsOverrideValidator(vmbda, h.client))
	}

	if provisioner != nil {
		overrideValidators = append(overrideValidators, restorer.NewProvisionerOverrideValidator(provisioner, h.client))
	}

	var toCreate []client.Object

	for _, ov := range overrideValidators {
		ov.Override(vmRestore.Spec.NameReplacements)

		err = ov.Validate(ctx)
		switch {
		case err == nil:
		case errors.Is(err, restorer.ErrAlreadyExists), errors.Is(err, restorer.ErrAlreadyInUse):
			vmRestore.Status.Phase = virtv2.VirtualMachineRestorePhaseFailed
			cb.
				Status(metav1.ConditionFalse).
				Reason(vmrestorecondition.VirtualMachineRestoreConflict).
				Message(service.CapitalizeFirstLetter(err.Error()) + ".")
			return reconcile.Result{}, nil
		default:
			setPhaseConditionToFailed(cb, &vmRestore.Status.Phase, err)
			return reconcile.Result{}, err
		}

		toCreate = append(toCreate, ov.Object())
	}

	err = h.createBatch(ctx, toCreate...)
	if err != nil {
		setPhaseConditionToFailed(cb, &vmRestore.Status.Phase, err)
		return reconcile.Result{}, err
	}

	vmRestore.Status.Phase = virtv2.VirtualMachineRestorePhaseInProgress
	cb.
		Status(metav1.ConditionFalse).
		Reason(vmrestorecondition.VirtualMachineSnapshotNotReady).
		Message(fmt.Sprintf("The virtual machine %q is in the process of restore.", vmSnapshot.Spec.VirtualMachineName))
	return reconcile.Result{Requeue: true}, nil
}

type OverrideValidator interface {
	Object() client.Object
	Override(rules []virtv2.NameReplacement)
	Validate(ctx context.Context) error
}

var ErrVirtualDiskSnapshotNotFound = errors.New("not found")

func (h LifeCycleHandler) getVirtualDisks(ctx context.Context, vmSnapshot *virtv2.VirtualMachineSnapshot) ([]*virtv2.VirtualDisk, error) {
	vds := make([]*virtv2.VirtualDisk, 0, len(vmSnapshot.Status.VirtualDiskSnapshotNames))

	for _, vdSnapshotName := range vmSnapshot.Status.VirtualDiskSnapshotNames {
		vdSnapshotKey := types.NamespacedName{Namespace: vmSnapshot.Namespace, Name: vdSnapshotName}
		vdSnapshot, err := object.FetchObject(ctx, vdSnapshotKey, h.client, &virtv2.VirtualDiskSnapshot{})
		if err != nil {
			return nil, fmt.Errorf("failed to fetch the virtual disk snapshot %q: %w", vdSnapshotKey.Name, err)
		}

		if vdSnapshot == nil {
			return nil, fmt.Errorf("the virtual disk snapshot %q %w", vdSnapshotName, ErrVirtualDiskSnapshotNotFound)
		}

		vd := virtv2.VirtualDisk{
			TypeMeta: metav1.TypeMeta{
				Kind:       virtv2.VirtualDiskKind,
				APIVersion: virtv2.Version,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      vdSnapshot.Spec.VirtualDiskName,
				Namespace: vdSnapshot.Namespace,
			},
			Spec: virtv2.VirtualDiskSpec{
				DataSource: &virtv2.VirtualDiskDataSource{
					Type: virtv2.DataSourceTypeObjectRef,
					ObjectRef: &virtv2.VirtualDiskObjectRef{
						Kind: virtv2.VirtualDiskObjectRefKindVirtualDiskSnapshot,
						Name: vdSnapshot.Name,
					},
				},
			},
		}

		vds = append(vds, &vd)
	}

	return vds, nil
}

func (h LifeCycleHandler) createBatch(ctx context.Context, objs ...client.Object) error {
	for _, obj := range objs {
		err := h.client.Create(ctx, obj)
		if err != nil && !k8serrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create %s %q: %w", obj.GetObjectKind().GroupVersionKind().Kind, obj.GetName(), err)
		}
	}

	return nil
}

func setPhaseConditionToFailed(cb *conditions.ConditionBuilder, phase *virtv2.VirtualMachineRestorePhase, err error) {
	*phase = virtv2.VirtualMachineRestorePhaseFailed
	cb.
		Status(metav1.ConditionFalse).
		Reason(vmrestorecondition.VirtualMachineRestoreFailed).
		Message(service.CapitalizeFirstLetter(err.Error()) + ".")
}
