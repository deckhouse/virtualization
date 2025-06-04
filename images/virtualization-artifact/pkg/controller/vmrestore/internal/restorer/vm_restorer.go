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
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const ReasonPVCNotFound = "PVC not found"

type VirtualMachineOverrideValidator struct {
	vm     *virtv2.VirtualMachine
	client client.Client
}

func NewVirtualMachineOverrideValidator(vmTmpl *virtv2.VirtualMachine, client client.Client) *VirtualMachineOverrideValidator {
	return &VirtualMachineOverrideValidator{
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
		client: client,
	}
}

func (v *VirtualMachineOverrideValidator) Override(rules []virtv2.NameReplacement) {
	v.vm.Name = overrideName(v.vm.Kind, v.vm.Name, rules)
	v.vm.Spec.VirtualMachineIPAddress = overrideName(virtv2.VirtualMachineIPAddressKind, v.vm.Spec.VirtualMachineIPAddress, rules)

	for i := range v.vm.Spec.BlockDeviceRefs {
		if v.vm.Spec.BlockDeviceRefs[i].Kind != virtv2.DiskDevice {
			continue
		}

		v.vm.Spec.BlockDeviceRefs[i].Name = overrideName(virtv2.VirtualDiskKind, v.vm.Spec.BlockDeviceRefs[i].Name, rules)
	}
}

func (v *VirtualMachineOverrideValidator) Validate(ctx context.Context) error {
	vmKey := types.NamespacedName{Namespace: v.vm.Namespace, Name: v.vm.Name}
	existed, err := object.FetchObject(ctx, vmKey, v.client, &virtv2.VirtualMachine{})
	if err != nil {
		return err
	}

	if existed != nil {
		return fmt.Errorf("the virtual machine %q %w", vmKey.Name, ErrAlreadyExists)
	}

	return nil
}

func (v *VirtualMachineOverrideValidator) ValidateWithForce(ctx context.Context) error {
	kvvmKey := types.NamespacedName{Name: v.vm.Name, Namespace: v.vm.Namespace}
	kvvm, err := object.FetchObject(ctx, kvvmKey, v.client, &virtv1.VirtualMachine{})
	if err != nil {
		return fmt.Errorf("failed to fetch the `InternalVirtualMachine`: %w", err)
	}

	if kvvm != nil {
		for _, vss := range kvvm.Status.VolumeSnapshotStatuses {
			if vss.Reason == ReasonPVCNotFound {
				return fmt.Errorf("waiting for the `VirtualDisks` %w", ErrRestoring)
			}
		}
	}

	return nil
}

func (v *VirtualMachineOverrideValidator) ProcessWithForce(ctx context.Context, vmRestoreUID string) error {
	vmKey := types.NamespacedName{Namespace: v.vm.Namespace, Name: v.vm.Name}
	vmObj, err := object.FetchObject(ctx, vmKey, v.client, &virtv2.VirtualMachine{})
	if err != nil {
		return fmt.Errorf("failed to fetch the `VirtualMachine`: %w", err)
	}

	if vmObj != nil {
		vmObj.SetAnnotations(map[string]string{annotations.AnnVMRestore: vmRestoreUID})
		if !equality.Semantic.DeepEqual(vmObj.Spec, v.vm.Spec) {
			vmObj.Spec = v.vm.Spec
		}
		updErr := v.client.Update(ctx, vmObj)
		if updErr != nil {
			if apierrors.IsConflict(updErr) {
				return fmt.Errorf("waiting for the `VirtualMachine` %w", ErrUpdating)
			} else {
				return fmt.Errorf("failed to update the `VirtualMachine`: %w", updErr)
			}
		}
	}

	return nil
}

func (v *VirtualMachineOverrideValidator) AnnotateObject(vmRestoreUID string) {
	if v.vm.Annotations != nil {
		v.vm.Annotations[annotations.AnnVMRestore] = vmRestoreUID
	} else {
		v.vm.Annotations = make(map[string]string)
		v.vm.Annotations[annotations.AnnVMRestore] = vmRestoreUID
	}
}

func (v *VirtualMachineOverrideValidator) Object() client.Object {
	return v.vm
}
