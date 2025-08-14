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

package restorer

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service/restorer/common"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const ReasonPVCNotFound = "PVC not found"

type VMHandler struct {
	vm         *virtv2.VirtualMachine
	client     client.Client
	restoreUID string
}

func NewVMHandler(vmTmpl *virtv2.VirtualMachine, client client.Client, vmopRestoreUID string) *VMHandler {
	if vmTmpl.Annotations != nil {
		vmTmpl.Annotations[annotations.AnnVMRestore] = vmopRestoreUID
	} else {
		vmTmpl.Annotations = make(map[string]string)
		vmTmpl.Annotations[annotations.AnnVMRestore] = vmopRestoreUID
	}

	vmTmpl.Spec.RunPolicy = virtv2.AlwaysOffPolicy

	return &VMHandler{
		vm: &virtv2.VirtualMachine{
			TypeMeta: metav1.TypeMeta{
				Kind:       vmTmpl.Kind,
				APIVersion: vmTmpl.APIVersion,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:        vmTmpl.Name,
				Namespace:   vmTmpl.Namespace,
				Annotations: vmTmpl.Annotations,
				Labels:      vmTmpl.Labels,
			},
			Spec: vmTmpl.Spec,
		},
		client:     client,
		restoreUID: vmopRestoreUID,
	}
}

func (v *VMHandler) Override(rules []virtv2.NameReplacement) {
	v.vm.Name = common.OverrideName(v.vm.Kind, v.vm.Name, rules)
	v.vm.Spec.VirtualMachineIPAddress = common.OverrideName(virtv2.VirtualMachineIPAddressKind, v.vm.Spec.VirtualMachineIPAddress, rules)

	if v.vm.Spec.Provisioning != nil {
		if v.vm.Spec.Provisioning.UserDataRef != nil {
			if v.vm.Spec.Provisioning.UserDataRef.Kind == virtv2.UserDataRefKindSecret {
				v.vm.Spec.Provisioning.UserDataRef.Name = common.OverrideName(
					string(virtv2.UserDataRefKindSecret),
					v.vm.Spec.Provisioning.UserDataRef.Name,
					rules,
				)
			}
		}
	}

	for i := range v.vm.Spec.BlockDeviceRefs {
		if v.vm.Spec.BlockDeviceRefs[i].Kind != virtv2.DiskDevice {
			continue
		}

		v.vm.Spec.BlockDeviceRefs[i].Name = common.OverrideName(virtv2.VirtualDiskKind, v.vm.Spec.BlockDeviceRefs[i].Name, rules)
	}
}

func (v *VMHandler) Validate(ctx context.Context) error {
	vmKey := types.NamespacedName{Namespace: v.vm.Namespace, Name: v.vm.Name}
	existed, err := object.FetchObject(ctx, vmKey, v.client, &virtv2.VirtualMachine{})
	if err != nil {
		return err
	}

	if existed != nil {
		return fmt.Errorf("the virtual machine %q %w", vmKey.Name, common.ErrAlreadyExists)
	}

	return nil
}

func (v *VMHandler) ValidateWithForce(ctx context.Context) error {
	return nil
}

func (v *VMHandler) Process(ctx context.Context) error {
	vmKey := types.NamespacedName{Namespace: v.vm.Namespace, Name: v.vm.Name}
	vmObj, err := object.FetchObject(ctx, vmKey, v.client, &virtv2.VirtualMachine{})
	if err != nil {
		return fmt.Errorf("failed to fetch the `VirtualMachine`: %w", err)
	}

	if vmObj != nil {
		var (
			vmHasCorrectVMRestoreUID bool
			vmHasVMSnapshotSpec      bool
		)

		if value, ok := vmObj.Annotations[annotations.AnnVMRestore]; !ok || value != v.restoreUID {
			vmHasCorrectVMRestoreUID = false
			vmObj.SetAnnotations(map[string]string{annotations.AnnVMRestore: v.restoreUID})
		}
		if !equality.Semantic.DeepEqual(vmObj.Spec, v.vm.Spec) {
			vmHasVMSnapshotSpec = false
			vmObj.Spec = v.vm.Spec
		}

		if !vmHasCorrectVMRestoreUID || !vmHasVMSnapshotSpec {
			updErr := v.client.Update(ctx, vmObj)
			if updErr != nil {
				if apierrors.IsConflict(updErr) {
					return fmt.Errorf("waiting for the `VirtualMachine` %w", common.ErrUpdating)
				} else {
					return fmt.Errorf("failed to update the `VirtualMachine`: %w", updErr)
				}
			}
		}
	}

	return nil
}

func (v *VMHandler) ProcessWithForce(ctx context.Context) error {
	vmKey := types.NamespacedName{Namespace: v.vm.Namespace, Name: v.vm.Name}
	vm, err := object.FetchObject(ctx, vmKey, v.client, &virtv2.VirtualMachine{})
	if err != nil {
		return fmt.Errorf("failed to fetch the `VirtualMachine`: %w", err)
	}

	err = v.deleteCurrentVirtualMachineBlockDeviceAttachments(ctx, vm.Name, vm.Namespace, v.restoreUID)
	if err != nil {
		return err
	}

	if vm != nil {
		var (
			vmHasCorrectVMRestoreUID bool
			vmHasVMSnapshotSpec      bool
		)

		if value, ok := vm.Annotations[annotations.AnnVMRestore]; !ok || value != v.restoreUID {
			vmHasCorrectVMRestoreUID = false
			vm.SetAnnotations(map[string]string{annotations.AnnVMRestore: v.restoreUID})
		}
		if !equality.Semantic.DeepEqual(vm.Spec, v.vm.Spec) {
			vmHasVMSnapshotSpec = false
			vm.Spec = v.vm.Spec
		}

		if !vmHasCorrectVMRestoreUID || !vmHasVMSnapshotSpec {
			updErr := v.client.Update(ctx, vm)
			if updErr != nil {
				if apierrors.IsConflict(updErr) {
					return fmt.Errorf("waiting for the `VirtualMachine` %w", common.ErrUpdating)
				} else {
					return fmt.Errorf("failed to update the `VirtualMachine`: %w", updErr)
				}
			}
		}
	}

	return nil
}

func (v *VMHandler) Object() client.Object {
	return v.vm
}

func (v *VMHandler) deleteCurrentVirtualMachineBlockDeviceAttachments(ctx context.Context, vmName, vmNamespace, vmRestoreUID string) error {
	vmbdas := &virtv2.VirtualMachineBlockDeviceAttachmentList{}
	err := v.client.List(ctx, vmbdas, &client.ListOptions{Namespace: vmNamespace})
	if err != nil {
		return fmt.Errorf("failed to list the `VirtualMachineBlockDeviceAttachment`: %w", err)
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

	for _, vmbda := range vmbdasByVM {
		err := object.DeleteObject(ctx, v.client, client.Object(vmbda))
		if err != nil {
			return fmt.Errorf("failed to delete the `VirtualMachineBlockDeviceAttachment` %s: %w", vmbda.Name, err)
		}
	}

	return nil
}
