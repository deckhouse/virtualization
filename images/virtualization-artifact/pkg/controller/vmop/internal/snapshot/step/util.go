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
	"errors"
	"fmt"
	"strings"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	kubeclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop/internal/snapshot/restorer"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	vmrestorecondition "github.com/deckhouse/virtualization/api/core/v1alpha2/vm-restore-condition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

var ErrVirtualDiskSnapshotNotFound = errors.New("not found")

const VDPrefix = "vd-"

func getVirtualDisks(ctx context.Context, client kubeclient.Client, vmSnapshot *virtv2.VirtualMachineSnapshot) ([]*virtv2.VirtualDisk, error) {
	vds := make([]*virtv2.VirtualDisk, 0, len(vmSnapshot.Status.VirtualDiskSnapshotNames))

	for _, vdSnapshotName := range vmSnapshot.Status.VirtualDiskSnapshotNames {
		vdSnapshotKey := types.NamespacedName{Namespace: vmSnapshot.Namespace, Name: vdSnapshotName}
		vdSnapshot, err := object.FetchObject(ctx, vdSnapshotKey, client, &virtv2.VirtualDiskSnapshot{})
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
			Status: virtv2.VirtualDiskStatus{
				AttachedToVirtualMachines: []virtv2.AttachedVirtualMachine{
					{Name: vmSnapshot.Spec.VirtualMachineName, Mounted: true},
				},
			},
		}

		vds = append(vds, &vd)
	}

	return vds, nil
}

func getCurrentVirtualMachineBlockDeviceAttachments(ctx context.Context, client kubeclient.Client, vmName, vmNamespace, vmRestoreUID string) ([]*virtv2.VirtualMachineBlockDeviceAttachment, error) {
	vmbdas := &virtv2.VirtualMachineBlockDeviceAttachmentList{}
	err := client.List(ctx, vmbdas, &kubeclient.ListOptions{Namespace: vmNamespace})
	if err != nil {
		return nil, fmt.Errorf("failed to list the `VirtualMachineBlockDeviceAttachment`: %w", err)
	}

	vmbdasByVM := make([]*virtv2.VirtualMachineBlockDeviceAttachment, 0, len(vmbdas.Items))
	for _, vmbda := range vmbdas.Items {
		if vmbda.Spec.VirtualMachineName != vmName {
			continue
		}
		if value, ok := vmbda.Annotations[annotations.AnnVMRestore]; ok && value == vmRestoreUID {
			continue
		}
		vmbdasByVM = append(vmbdasByVM, &vmbda)
	}

	return vmbdasByVM, nil
}

func deleteCurrentVirtualMachineBlockDeviceAttachments(ctx context.Context, client kubeclient.Client, vmbdas []*virtv2.VirtualMachineBlockDeviceAttachment) error {
	for _, vmbda := range vmbdas {
		err := object.DeleteObject(ctx, client, kubeclient.Object(vmbda))
		if err != nil {
			return fmt.Errorf("failed to delete the `VirtualMachineBlockDeviceAttachment` %s: %w", vmbda.Name, err)
		}
	}

	return nil
}

func createBatch(ctx context.Context, client kubeclient.Client, objs ...kubeclient.Object) error {
	for _, obj := range objs {
		err := client.Create(ctx, obj)
		if err != nil && !k8serrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create %s %q: %w", obj.GetObjectKind().GroupVersionKind().Kind, obj.GetName(), err)
		}
	}

	return nil
}

func setPhaseConditionToFailed(cb *conditions.ConditionBuilder, phase *virtv2.VMOPPhase, err error) {
	*phase = virtv2.VMOPPhaseFailed
	cb.
		Status(metav1.ConditionFalse).
		Reason(vmrestorecondition.VirtualMachineRestoreFailed).
		Message(service.CapitalizeFirstLetter(err.Error()) + ".")
}

func setPhaseConditionToPending(cb *conditions.ConditionBuilder, phase *virtv2.VMOPPhase, reason vmopcondition.ReasonCompleted, msg string) {
	*phase = virtv2.VMOPPhasePending
	cb.
		Status(metav1.ConditionFalse).
		Reason(reason).
		Message(service.CapitalizeFirstLetter(msg) + ".")
}

func NewVMRestoreVMOP(vmName, namespace, vmRestoreUID string, vmopType virtv2.VMOPType) *virtv2.VirtualMachineOperation {
	return vmopbuilder.New(
		vmopbuilder.WithGenerateName("vmrestore-"),
		vmopbuilder.WithNamespace(namespace),
		vmopbuilder.WithAnnotation(annotations.AnnVMRestore, vmRestoreUID),
		vmopbuilder.WithType(vmopType),
		vmopbuilder.WithVirtualMachine(vmName),
	)
}

func GetVMRestoreVMOP(ctx context.Context, client kubeclient.Client, vmNamespace, vmRestoreUID string, vmopType virtv2.VMOPType) (*virtv2.VirtualMachineOperation, error) {
	vmops := &virtv2.VirtualMachineOperationList{}
	err := client.List(ctx, vmops, &kubeclient.ListOptions{Namespace: vmNamespace})
	if err != nil {
		return nil, err
	}

	for _, vmop := range vmops.Items {
		if v, ok := vmop.Annotations[annotations.AnnVMRestore]; ok {
			if v == vmRestoreUID && vmop.Spec.Type == vmopType {
				return &vmop, nil
			}
		}
	}

	return nil, nil
}

func stopVirtualMachine(ctx context.Context, client kubeclient.Client, vmName, vmNamespace, vmRestoreUID string) error {
	vmopStop, err := GetVMRestoreVMOP(ctx, client, vmNamespace, vmRestoreUID, virtv2.VMOPTypeStop)
	if err != nil {
		return fmt.Errorf("failed to list the `VirtualMachineOperations`: %w", err)
	}

	if vmopStop == nil {
		vmopStop := NewVMRestoreVMOP(vmName, vmNamespace, vmRestoreUID, virtv2.VMOPTypeStop)
		err := client.Create(ctx, vmopStop)
		if err != nil {
			return fmt.Errorf("failed to stop the `VirtualMachine`: %w", err)
		}
		return fmt.Errorf("the status of the virtual machine operation is %w", restorer.ErrIncomplete)
	}

	conditionCompleted, _ := conditions.GetCondition(vmopcondition.TypeCompleted, vmopStop.Status.Conditions)
	switch vmopStop.Status.Phase {
	case virtv2.VMOPPhaseFailed:
		return fmt.Errorf("failed to stop the `VirtualMachine`: %s", conditionCompleted.Message)
	case virtv2.VMOPPhaseCompleted:
		return nil
	default:
		return fmt.Errorf("the status of the `VirtualMachineOperation` is %w: %s", restorer.ErrIncomplete, conditionCompleted.Message)
	}
}

func startVirtualMachine(ctx context.Context, client kubeclient.Client, vmop *virtv2.VirtualMachineOperation) error {
	vms := &virtv2.VirtualMachineList{}
	err := client.List(ctx, vms, &kubeclient.ListOptions{Namespace: vmop.Namespace})
	if err != nil {
		return fmt.Errorf("failed to list the `VirtualMachines`: %w", err)
	}

	var vmName string
	for _, vm := range vms.Items {
		if v, ok := vm.Annotations[annotations.AnnVMRestore]; ok && v == string(vmop.UID) {
			vmName = vm.Name
		}
	}

	vmKey := types.NamespacedName{Name: vmName, Namespace: vmop.Namespace}
	vmObj, err := object.FetchObject(ctx, vmKey, client, &virtv2.VirtualMachine{})
	if err != nil {
		return fmt.Errorf("failed to fetch the `VirtualMachine`: %w", err)
	}

	if vmObj != nil {
		if vmObj.Spec.RunPolicy == virtv2.AlwaysOffPolicy || vmObj.Spec.RunPolicy == virtv2.ManualPolicy {
			return nil
		}

		if vmObj.Status.Phase == virtv2.MachineStopped {
			vmopStart := NewVMRestoreVMOP(vmName, vmop.Namespace, string(vmop.UID), virtv2.VMOPTypeStart)
			err := client.Create(ctx, vmopStart)
			if err != nil {
				return fmt.Errorf("failed to start the `VirtualMachine`: %w", err)
			}
		}
	}

	return nil
}

func checkKVVMDiskStatus(ctx context.Context, client kubeclient.Client, vmName, vmNamespace string) error {
	kvvmKey := types.NamespacedName{Name: vmName, Namespace: vmNamespace}
	kvvm, err := object.FetchObject(ctx, kvvmKey, client, &virtv1.VirtualMachine{})
	if err != nil {
		return fmt.Errorf("failed to fetch the `InternalVirtualMachine`: %w", err)
	}

	if kvvm != nil {
		for _, vss := range kvvm.Status.VolumeSnapshotStatuses {
			if strings.HasPrefix(vss.Name, VDPrefix) && vss.Reason == restorer.ReasonPVCNotFound {
				return fmt.Errorf("waiting for the `VirtualDisks` %w", restorer.ErrRestoring)
			}
		}
		return nil
	}

	return fmt.Errorf("failed to fetch the `InternalVirtualMachine`: %s", vmName)
}

type OverrideValidator interface {
	Object() kubeclient.Object
	Override(rules []virtv2.NameReplacement)
	Validate(ctx context.Context) error
	ValidateWithForce(ctx context.Context) error
	ProcessWithForce(ctx context.Context) error
}

func getOverrridedVMName(overrideValidators []OverrideValidator) (string, error) {
	for _, ov := range overrideValidators {
		if ov.Object().GetObjectKind().GroupVersionKind().Kind == virtv2.VirtualMachineKind {
			return ov.Object().GetName(), nil
		}
	}

	return "", fmt.Errorf("failed to get the `VirtualMachine` name")
}
