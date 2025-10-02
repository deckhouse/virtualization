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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VirtualMachineBlockDeviceAttachmentsOverrideValidator struct {
	vmbda        *v1alpha2.VirtualMachineBlockDeviceAttachment
	client       client.Client
	vmRestoreUID string
}

func NewVirtualMachineBlockDeviceAttachmentsOverrideValidator(vmbdaTmpl *v1alpha2.VirtualMachineBlockDeviceAttachment, client client.Client, vmRestoreUID string) *VirtualMachineBlockDeviceAttachmentsOverrideValidator {
	if vmbdaTmpl.Annotations != nil {
		vmbdaTmpl.Annotations[annotations.AnnVMRestore] = vmRestoreUID
	} else {
		vmbdaTmpl.Annotations = make(map[string]string)
		vmbdaTmpl.Annotations[annotations.AnnVMRestore] = vmRestoreUID
	}
	return &VirtualMachineBlockDeviceAttachmentsOverrideValidator{
		vmbda: &v1alpha2.VirtualMachineBlockDeviceAttachment{
			TypeMeta: metav1.TypeMeta{
				Kind:       vmbdaTmpl.Kind,
				APIVersion: vmbdaTmpl.APIVersion,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:        vmbdaTmpl.Name,
				Namespace:   vmbdaTmpl.Namespace,
				Annotations: vmbdaTmpl.Annotations,
				Labels:      vmbdaTmpl.Labels,
			},
			Spec: vmbdaTmpl.Spec,
		},
		client:       client,
		vmRestoreUID: vmRestoreUID,
	}
}

func (v *VirtualMachineBlockDeviceAttachmentsOverrideValidator) Override(rules []v1alpha2.NameReplacement) {
	v.vmbda.Name = overrideName(v.vmbda.Kind, v.vmbda.Name, rules)
	v.vmbda.Spec.VirtualMachineName = overrideName(v1alpha2.VirtualMachineKind, v.vmbda.Spec.VirtualMachineName, rules)

	switch v.vmbda.Spec.BlockDeviceRef.Kind {
	case v1alpha2.VMBDAObjectRefKindVirtualDisk:
		v.vmbda.Spec.BlockDeviceRef.Name = overrideName(v1alpha2.VirtualDiskKind, v.vmbda.Spec.BlockDeviceRef.Name, rules)
	case v1alpha2.VMBDAObjectRefKindClusterVirtualImage:
		v.vmbda.Spec.BlockDeviceRef.Name = overrideName(v1alpha2.ClusterVirtualImageKind, v.vmbda.Spec.BlockDeviceRef.Name, rules)
	case v1alpha2.VMBDAObjectRefKindVirtualImage:
		v.vmbda.Spec.BlockDeviceRef.Name = overrideName(v1alpha2.VirtualImageKind, v.vmbda.Spec.BlockDeviceRef.Name, rules)
	}
}

func (v *VirtualMachineBlockDeviceAttachmentsOverrideValidator) Validate(ctx context.Context) error {
	vmbdaKey := types.NamespacedName{Namespace: v.vmbda.Namespace, Name: v.vmbda.Name}
	existed, err := object.FetchObject(ctx, vmbdaKey, v.client, &v1alpha2.VirtualMachineBlockDeviceAttachment{})
	if err != nil {
		return err
	}

	if existed != nil {
		if value, ok := existed.Annotations[annotations.AnnVMRestore]; ok && value == v.vmRestoreUID {
			return nil
		}
		return fmt.Errorf("the virtual machine block device attachment %q %w", vmbdaKey.Name, ErrAlreadyExists)
	}

	return nil
}

func (v *VirtualMachineBlockDeviceAttachmentsOverrideValidator) ValidateWithForce(ctx context.Context) error {
	return nil
}

func (v *VirtualMachineBlockDeviceAttachmentsOverrideValidator) ProcessWithForce(ctx context.Context) error {
	vmbdaKey := types.NamespacedName{Namespace: v.vmbda.Namespace, Name: v.vmbda.Name}
	vmbdaObj, err := object.FetchObject(ctx, vmbdaKey, v.client, &v1alpha2.VirtualMachineBlockDeviceAttachment{})
	if err != nil {
		return fmt.Errorf("failed to fetch the `VirtualMachineBlockDeviceAttachment`: %w", err)
	}

	if object.IsTerminating(vmbdaObj) {
		return fmt.Errorf("waiting for the `VirtualMachineBlockDeviceAttachment` %s %w", vmbdaObj.Name, ErrRestoring)
	}

	if vmbdaObj != nil {
		if value, ok := vmbdaObj.Annotations[annotations.AnnVMRestore]; ok && value == v.vmRestoreUID {
			return nil
		}
		err = v.client.Delete(ctx, vmbdaObj)
		if err != nil {
			return fmt.Errorf("failed to delete the `VirtualMachineBlockDeviceAttachment`: %w", err)
		}
		return fmt.Errorf("waiting for the `VirtualMachineBlockDeviceAttachment` %s %w", vmbdaObj.Name, ErrRestoring)
	}

	return nil
}

func (v *VirtualMachineBlockDeviceAttachmentsOverrideValidator) Object() client.Object {
	return v.vmbda
}
