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

	corev1 "k8s.io/api/core/v1"
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
	mode       common.OperationMode
}

func NewVirtualMachineHandler(client client.Client, vmTmpl v1alpha2.VirtualMachine, vmopRestoreUID string, mode common.OperationMode) *VirtualMachineHandler {
	if vmTmpl.Annotations != nil {
		vmTmpl.Annotations[annotations.AnnVMRestore] = vmopRestoreUID
	} else {
		vmTmpl.Annotations = make(map[string]string)
		vmTmpl.Annotations[annotations.AnnVMRestore] = vmopRestoreUID
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

func (v *VirtualMachineHandler) ValidateRestore(ctx context.Context) error {
	vmKey := types.NamespacedName{Namespace: v.vm.Namespace, Name: v.vm.Name}
	existed, err := object.FetchObject(ctx, vmKey, v.client, &v1alpha2.VirtualMachine{})
	if err != nil {
		return err
	}

	if existed != nil {
		if value, ok := existed.Annotations[annotations.AnnVMRestore]; ok && value == v.restoreUID {
			return nil
		}

		cond, found := conditions.GetCondition(vmcondition.TypeMaintenance, existed.Status.Conditions)
		if !found {
			return common.ErrVMMaintenanceCondNotFound
		}

		if cond.Status != metav1.ConditionTrue {
			return common.ErrVMNotInMaintenance
		}
	}

	if err := v.validateImageDependencies(ctx); err != nil {
		return err
	}

	if err := v.validateProvisionerDependencies(ctx); err != nil {
		return err
	}

	return nil
}

func (v *VirtualMachineHandler) ValidateClone(ctx context.Context) error {
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
	if err := v.validateProvisionerDependencies(ctx); err != nil {
		return err
	}

	vmKey := types.NamespacedName{Namespace: v.vm.Namespace, Name: v.vm.Name}
	vm, err := object.FetchObject(ctx, vmKey, v.client, &v1alpha2.VirtualMachine{})
	if err != nil {
		return fmt.Errorf("failed to fetch the `VirtualMachine`: %w", err)
	}

	if vm != nil {
		// Always clean up VMBDAs first, regardless of VM state
		err = v.deleteCurrentVirtualMachineBlockDeviceAttachments(ctx, vm.Name, vm.Namespace, v.restoreUID)
		if err != nil {
			return err
		}

		// Early return if VM is already fully processed by this restore operation
		if value, ok := vm.Annotations[annotations.AnnVMRestore]; ok && value == v.restoreUID {
			if equality.Semantic.DeepEqual(vm.Spec, v.vm.Spec) {
				return nil
			}
		}

		var (
			vmHasCorrectVMRestoreUID = true
			vmHasVMSnapshotSpec      = true
		)

		if value, ok := vm.Annotations[annotations.AnnVMRestore]; !ok || value != v.restoreUID {
			vmHasCorrectVMRestoreUID = false
			if vm.Annotations == nil {
				vm.Annotations = make(map[string]string)
			}
			vm.Annotations[annotations.AnnVMRestore] = v.restoreUID
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
	} else {
		err := v.client.Create(ctx, v.vm)
		if err != nil {
			return fmt.Errorf("failed to create the `VirtualMachine`: %w", err)
		}
	}

	return nil
}

func (v *VirtualMachineHandler) ProcessClone(ctx context.Context) error {
	return nil
}

func (v *VirtualMachineHandler) validateImageDependencies(ctx context.Context) error {
	var indicesToRemove []int

	for i, ref := range v.vm.Spec.BlockDeviceRefs {
		var missing bool
		var err error

		switch ref.Kind {
		case v1alpha2.ImageDevice:
			missing, err = v.validateVirtualImageRef(ctx, &ref)
		case v1alpha2.ClusterImageDevice:
			missing, err = v.validateClusterVirtualImageRef(ctx, &ref)
		default:
			continue
		}

		if err != nil {
			return err
		}
		if missing {
			indicesToRemove = append(indicesToRemove, i)
		}
	}

	for i := len(indicesToRemove) - 1; i >= 0; i-- {
		idx := indicesToRemove[i]
		v.vm.Spec.BlockDeviceRefs = append(v.vm.Spec.BlockDeviceRefs[:idx], v.vm.Spec.BlockDeviceRefs[idx+1:]...)
	}

	return nil
}

func (v *VirtualMachineHandler) validateVirtualImageRef(ctx context.Context, ref *v1alpha2.BlockDeviceSpecRef) (bool, error) {
	key := types.NamespacedName{Namespace: v.vm.Namespace, Name: ref.Name}
	obj, err := object.FetchObject(ctx, key, v.client, &v1alpha2.VirtualImage{})
	return v.handleMissingResource(obj, err, ref.Kind, ref.Name)
}

func (v *VirtualMachineHandler) validateClusterVirtualImageRef(ctx context.Context, ref *v1alpha2.BlockDeviceSpecRef) (bool, error) {
	key := types.NamespacedName{Name: ref.Name}
	obj, err := object.FetchObject(ctx, key, v.client, &v1alpha2.ClusterVirtualImage{})
	return v.handleMissingResource(obj, err, ref.Kind, ref.Name)
}

func (v *VirtualMachineHandler) handleMissingResource(obj client.Object, err error, resourceType v1alpha2.BlockDeviceKind, name string) (bool, error) {
	if err != nil {
		return false, err
	}
	if obj == nil {
		if v.mode == common.BestEffortRestorerMode {
			return true, nil
		}
		return false, fmt.Errorf("%s %q not found", resourceType, name)
	}
	return false, nil
}

func (v *VirtualMachineHandler) validateProvisionerDependencies(ctx context.Context) error {
	if v.vm.Spec.Provisioning == nil || v.vm.Spec.Provisioning.UserDataRef == nil ||
		v.vm.Spec.Provisioning.UserDataRef.Kind != v1alpha2.UserDataRefKindSecret {
		return nil
	}

	userDataRef := v.vm.Spec.Provisioning.UserDataRef
	key := types.NamespacedName{Namespace: v.vm.Namespace, Name: userDataRef.Name}
	secret, err := object.FetchObject(ctx, key, v.client, &corev1.Secret{})
	if err != nil {
		return err
	}

	if secret == nil {
		if v.mode == common.BestEffortRestorerMode {
			v.vm.Spec.Provisioning.UserDataRef = nil
		} else {
			return fmt.Errorf("provisioner secret %q not found", userDataRef.Name)
		}
	}

	return nil
}

func (v *VirtualMachineHandler) Object() client.Object {
	return v.vm
}

func (v *VirtualMachineHandler) deleteCurrentVirtualMachineBlockDeviceAttachments(ctx context.Context, vmName, vmNamespace, vmRestoreUID string) error {
	vmbdas := &v1alpha2.VirtualMachineBlockDeviceAttachmentList{}
	err := v.client.List(ctx, vmbdas, &client.ListOptions{Namespace: vmNamespace})
	if err != nil {
		return fmt.Errorf("failed to list the `VirtualMachineBlockDeviceAttachment`: %w", err)
	}

	vmbdasByVM := make([]*v1alpha2.VirtualMachineBlockDeviceAttachment, 0, len(vmbdas.Items))
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
