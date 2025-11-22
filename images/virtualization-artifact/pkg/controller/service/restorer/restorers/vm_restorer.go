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
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service/restorer/common"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

const ReasonPVCNotFound = "PVC not found"

type VirtualMachineHandler struct {
	vm         *v1alpha2.VirtualMachine
	client     client.Client
	restoreUID string
	mode       v1alpha2.SnapshotOperationMode
}

func NewVirtualMachineHandler(client client.Client, vmTmpl v1alpha2.VirtualMachine, vmopRestoreUID string, mode v1alpha2.SnapshotOperationMode) *VirtualMachineHandler {
	if vmTmpl.Annotations != nil {
		vmTmpl.Annotations[annotations.AnnVMOPRestore] = vmopRestoreUID
	} else {
		vmTmpl.Annotations = make(map[string]string)
		vmTmpl.Annotations[annotations.AnnVMOPRestore] = vmopRestoreUID
	}

	return &VirtualMachineHandler{
		vm: &v1alpha2.VirtualMachine{
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
		mode:       mode,
	}
}

func (v *VirtualMachineHandler) Override(rules []v1alpha2.NameReplacement) {
	v.vm.Name = common.OverrideName(v.vm.Kind, v.vm.Name, rules)
	v.vm.Spec.VirtualMachineIPAddress = common.OverrideName(v1alpha2.VirtualMachineIPAddressKind, v.vm.Spec.VirtualMachineIPAddress, rules)

	if v.vm.Spec.Provisioning != nil {
		if v.vm.Spec.Provisioning.UserDataRef != nil {
			if v.vm.Spec.Provisioning.UserDataRef.Kind == v1alpha2.UserDataRefKindSecret {
				v.vm.Spec.Provisioning.UserDataRef.Name = common.OverrideName(
					string(v1alpha2.UserDataRefKindSecret),
					v.vm.Spec.Provisioning.UserDataRef.Name,
					rules,
				)
			}
		}
	}

	for i := range v.vm.Spec.BlockDeviceRefs {
		if v.vm.Spec.BlockDeviceRefs[i].Kind != v1alpha2.DiskDevice {
			continue
		}

		v.vm.Spec.BlockDeviceRefs[i].Name = common.OverrideName(v1alpha2.VirtualDiskKind, v.vm.Spec.BlockDeviceRefs[i].Name, rules)
	}
}

func (v *VirtualMachineHandler) Customize(prefix, suffix string) {
	v.vm.Name = common.ApplyNameCustomization(v.vm.Name, prefix, suffix)

	if v.vm.Spec.VirtualMachineIPAddress != "" {
		v.vm.Spec.VirtualMachineIPAddress = common.ApplyNameCustomization(v.vm.Spec.VirtualMachineIPAddress, prefix, suffix)
	}

	if v.vm.Spec.Provisioning != nil && v.vm.Spec.Provisioning.UserDataRef != nil {
		if v.vm.Spec.Provisioning.UserDataRef.Kind == v1alpha2.UserDataRefKindSecret {
			v.vm.Spec.Provisioning.UserDataRef.Name = common.ApplyNameCustomization(v.vm.Spec.Provisioning.UserDataRef.Name, prefix, suffix)
		}
	}

	for i := range v.vm.Spec.BlockDeviceRefs {
		if v.vm.Spec.BlockDeviceRefs[i].Kind != v1alpha2.DiskDevice {
			continue
		}
		v.vm.Spec.BlockDeviceRefs[i].Name = common.ApplyNameCustomization(v.vm.Spec.BlockDeviceRefs[i].Name, prefix, suffix)
	}
}

func (v *VirtualMachineHandler) ValidateRestore(ctx context.Context) error {
	vmKey := types.NamespacedName{Namespace: v.vm.Namespace, Name: v.vm.Name}
	existed, err := object.FetchObject(ctx, vmKey, v.client, &v1alpha2.VirtualMachine{})
	if err != nil {
		return err
	}

	if existed != nil {
		if value, ok := existed.Annotations[annotations.AnnVMOPRestore]; ok && value == v.restoreUID {
			return nil
		}
	}

	if err := v.validateImageDependencies(ctx); err != nil {
		return err
	}

	return nil
}

func (v *VirtualMachineHandler) ValidateClone(ctx context.Context) error {
	if err := common.ValidateResourceNameLength(v.vm.Name, v.vm.Kind); err != nil {
		return err
	}

	vmKey := types.NamespacedName{Namespace: v.vm.Namespace, Name: v.vm.Name}
	existed, err := object.FetchObject(ctx, vmKey, v.client, &v1alpha2.VirtualMachine{})
	if err != nil {
		return err
	}

	if existed != nil {
		if value, ok := existed.Annotations[annotations.AnnVMOPRestore]; ok && value == v.restoreUID {
			return nil
		}

		return fmt.Errorf("VirtualMachine with name %s already exists", v.vm.Name)
	}

	if err := v.validateImageDependencies(ctx); err != nil {
		return err
	}

	return nil
}

func (v *VirtualMachineHandler) ProcessRestore(ctx context.Context) error {
	err := v.ValidateRestore(ctx)
	if err != nil {
		return err
	}

	if err := v.validateImageDependencies(ctx); err != nil {
		return err
	}

	vmKey := types.NamespacedName{Namespace: v.vm.Namespace, Name: v.vm.Name}
	vm, err := object.FetchObject(ctx, vmKey, v.client, &v1alpha2.VirtualMachine{})
	if err != nil {
		return fmt.Errorf("failed to fetch the `VirtualMachine`: %w", err)
	}

	if vm != nil {
		cond, found := conditions.GetCondition(vmcondition.TypeMaintenance, vm.Status.Conditions)
		if !found {
			return common.ErrVMMaintenanceCondNotFound
		}

		if cond.Status != metav1.ConditionTrue {
			return common.ErrVMNotInMaintenance
		}

		// Early return if VM is already fully processed by this restore operation
		if value, ok := vm.Annotations[annotations.AnnVMOPRestore]; ok && value == v.restoreUID {
			if equality.Semantic.DeepEqual(vm.Spec, v.vm.Spec) {
				return nil
			}
		}

		if vm.Annotations == nil {
			vm.Annotations = make(map[string]string)
		}

		vm.Spec = v.vm.Spec
		vm.Labels = v.vm.Labels
		vm.Annotations = v.vm.Annotations
		vm.Annotations[annotations.AnnVMOPRestore] = v.restoreUID

		updErr := v.client.Update(ctx, vm)
		if updErr != nil {
			if apierrors.IsConflict(updErr) {
				return fmt.Errorf("waiting for the `VirtualMachine` %w", common.ErrUpdating)
			}
			return fmt.Errorf("failed to update the `VirtualMachine`: %w", updErr)
		}

		// Always clean up VMBDAs first, regardless of VM state
		err = v.deleteCurrentVirtualMachineBlockDeviceAttachments(ctx)
		if err != nil {
			return err
		}
	} else {
		err := v.client.Create(ctx, v.vm)
		if err != nil {
			return fmt.Errorf("failed to create the `VirtualMachine`: %w", err)
		}
	}

	return nil
}

func (v *VirtualMachineHandler) ProcessClone(ctx context.Context) error {
	err := v.ValidateClone(ctx)
	if err != nil {
		return err
	}

	vmKey := types.NamespacedName{Namespace: v.vm.Namespace, Name: v.vm.Name}
	existed, err := object.FetchObject(ctx, vmKey, v.client, &v1alpha2.VirtualMachine{})
	if err != nil {
		return err
	}

	if existed != nil {
		if value, ok := existed.Annotations[annotations.AnnVMOPRestore]; ok && value == v.restoreUID {
			return nil
		}
	}

	if err := v.validateImageDependencies(ctx); err != nil {
		return err
	}

	err = v.client.Create(ctx, v.vm)
	if err != nil {
		return fmt.Errorf("failed to create the `VirtualMachine`: %w", err)
	}

	return nil
}

func (v *VirtualMachineHandler) validateImageDependencies(ctx context.Context) error {
	filteredRefs := make([]v1alpha2.BlockDeviceSpecRef, 0, len(v.vm.Spec.BlockDeviceRefs))

	for _, ref := range v.vm.Spec.BlockDeviceRefs {
		if ref.Kind != v1alpha2.ImageDevice && ref.Kind != v1alpha2.ClusterImageDevice {
			filteredRefs = append(filteredRefs, ref)
			continue
		}

		if v.mode == v1alpha2.SnapshotOperationModeStrict {
			filteredRefs = append(filteredRefs, ref)
			continue
		}

		exists, err := v.imageExists(ctx, ref)
		if err != nil {
			return err
		}

		if exists {
			filteredRefs = append(filteredRefs, ref)
		}
	}

	v.vm.Spec.BlockDeviceRefs = filteredRefs
	return nil
}

func (v *VirtualMachineHandler) imageExists(ctx context.Context, ref v1alpha2.BlockDeviceSpecRef) (bool, error) {
	var obj client.Object
	var key types.NamespacedName

	switch ref.Kind {
	case v1alpha2.ImageDevice:
		obj = &v1alpha2.VirtualImage{}
		key = types.NamespacedName{Namespace: v.vm.Namespace, Name: ref.Name}
	case v1alpha2.ClusterImageDevice:
		obj = &v1alpha2.ClusterVirtualImage{}
		key = types.NamespacedName{Name: ref.Name}
	default:
		return true, nil
	}

	fetchedObj, err := object.FetchObject(ctx, key, v.client, obj)
	if err != nil {
		return false, err
	}

	if fetchedObj == nil {
		return false, nil
	}

	return true, nil
}

func (v *VirtualMachineHandler) Object() client.Object {
	return v.vm
}

func (v *VirtualMachineHandler) deleteCurrentVirtualMachineBlockDeviceAttachments(ctx context.Context) error {
	vmbdas := &v1alpha2.VirtualMachineBlockDeviceAttachmentList{}
	err := v.client.List(ctx, vmbdas, &client.ListOptions{Namespace: v.vm.Namespace})
	if err != nil {
		return fmt.Errorf("failed to list the `VirtualMachineBlockDeviceAttachment`: %w", err)
	}

	vmbdasToDelete := make([]*v1alpha2.VirtualMachineBlockDeviceAttachment, 0, len(vmbdas.Items))
	for _, vmbda := range vmbdas.Items {
		if vmbda.Spec.VirtualMachineName != v.vm.Name {
			continue
		}

		if value, ok := vmbda.Annotations[annotations.AnnVMOPRestore]; !ok || value != v.restoreUID {
			vmbdasToDelete = append(vmbdasToDelete, &vmbda)
		}
	}

	for _, vmbda := range vmbdasToDelete {
		err := object.DeleteObject(ctx, v.client, client.Object(vmbda))
		if err != nil {
			return fmt.Errorf("failed to delete the `VirtualMachineBlockDeviceAttachment` %s: %w", vmbda.Name, err)
		}
	}

	return nil
}
