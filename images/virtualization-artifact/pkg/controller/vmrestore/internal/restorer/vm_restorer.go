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
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const ReasonPVCNotFound = "PVC not found"

type VirtualMachineOverrideValidator struct {
	vm           *v1alpha2.VirtualMachine
	client       client.Client
	vmRestoreUID string
}

func NewVirtualMachineOverrideValidator(vmTmpl *v1alpha2.VirtualMachine, client client.Client, vmRestoreUID string) *VirtualMachineOverrideValidator {
	if vmTmpl.Annotations != nil {
		vmTmpl.Annotations[annotations.AnnVMRestore] = vmRestoreUID
	} else {
		vmTmpl.Annotations = make(map[string]string)
		vmTmpl.Annotations[annotations.AnnVMRestore] = vmRestoreUID
	}

	return &VirtualMachineOverrideValidator{
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
		client:       client,
		vmRestoreUID: vmRestoreUID,
	}
}

func (v *VirtualMachineOverrideValidator) Override(rules []v1alpha2.NameReplacement) {
	originalName := v.vm.Name
	v.vm.Name = overrideName(v.vm.Kind, v.vm.Name, rules)
	if v.vm.Name != originalName {
		// Do not clone labels due to potential issues with traffic from services.
		v.vm.Labels = map[string]string{}
	}

	v.vm.Spec.VirtualMachineIPAddress = overrideName(v1alpha2.VirtualMachineIPAddressKind, v.vm.Spec.VirtualMachineIPAddress, rules)

	if v.vm.Spec.Provisioning != nil {
		if v.vm.Spec.Provisioning.UserDataRef != nil {
			if v.vm.Spec.Provisioning.UserDataRef.Kind == v1alpha2.UserDataRefKindSecret {
				v.vm.Spec.Provisioning.UserDataRef.Name = overrideName(
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

		v.vm.Spec.BlockDeviceRefs[i].Name = overrideName(v1alpha2.VirtualDiskKind, v.vm.Spec.BlockDeviceRefs[i].Name, rules)
	}
}

func (v *VirtualMachineOverrideValidator) Validate(ctx context.Context) error {
	vmKey := types.NamespacedName{Namespace: v.vm.Namespace, Name: v.vm.Name}
	existed, err := object.FetchObject(ctx, vmKey, v.client, &v1alpha2.VirtualMachine{})
	if err != nil {
		return err
	}

	if existed != nil {
		if value, ok := existed.Annotations[annotations.AnnVMRestore]; ok && value == v.vmRestoreUID {
			return nil
		}
		return fmt.Errorf("the virtual machine %q %w", vmKey.Name, ErrAlreadyExists)
	}

	return nil
}

func (v *VirtualMachineOverrideValidator) ValidateWithForce(ctx context.Context) error {
	return nil
}

func (v *VirtualMachineOverrideValidator) ProcessWithForce(ctx context.Context) error {
	vmKey := types.NamespacedName{Namespace: v.vm.Namespace, Name: v.vm.Name}
	vmObj, err := object.FetchObject(ctx, vmKey, v.client, &v1alpha2.VirtualMachine{})
	if err != nil {
		return fmt.Errorf("failed to fetch the `VirtualMachine`: %w", err)
	}

	if vmObj != nil {
		var (
			vmHasCorrectVMRestoreUID bool
			vmHasVMSnapshotSpec      bool
		)

		if value, ok := vmObj.Annotations[annotations.AnnVMRestore]; !ok || value != v.vmRestoreUID {
			vmHasCorrectVMRestoreUID = false
			vmObj.SetAnnotations(map[string]string{annotations.AnnVMRestore: v.vmRestoreUID})
		}
		if !equality.Semantic.DeepEqual(vmObj.Spec, v.vm.Spec) {
			vmHasVMSnapshotSpec = false
			vmObj.Spec = v.vm.Spec
		}

		if !vmHasCorrectVMRestoreUID || !vmHasVMSnapshotSpec {
			updErr := v.client.Update(ctx, vmObj)
			if updErr != nil {
				if apierrors.IsConflict(updErr) {
					return fmt.Errorf("waiting for the `VirtualMachine` %w", ErrUpdating)
				}
				return fmt.Errorf("failed to update the `VirtualMachine`: %w", updErr)
			}
		}
	}

	return nil
}

func (v *VirtualMachineOverrideValidator) Object() client.Object {
	return v.vm
}
