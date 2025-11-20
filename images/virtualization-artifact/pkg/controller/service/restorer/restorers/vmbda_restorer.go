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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service/restorer/common"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VMBlockDeviceAttachmentHandler struct {
	vmbda      *v1alpha2.VirtualMachineBlockDeviceAttachment
	client     client.Client
	restoreUID string
}

func NewVMBlockDeviceAttachmentHandler(client client.Client, vmbdaTmpl v1alpha2.VirtualMachineBlockDeviceAttachment, vmRestoreUID string) *VMBlockDeviceAttachmentHandler {
	if vmbdaTmpl.Annotations != nil {
		vmbdaTmpl.Annotations[annotations.AnnVMOPRestore] = vmRestoreUID
	} else {
		vmbdaTmpl.Annotations = make(map[string]string)
		vmbdaTmpl.Annotations[annotations.AnnVMOPRestore] = vmRestoreUID
	}
	return &VMBlockDeviceAttachmentHandler{
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
		client:     client,
		restoreUID: vmRestoreUID,
	}
}

func (v *VMBlockDeviceAttachmentHandler) Override(rules []v1alpha2.NameReplacement) {
	v.vmbda.Name = common.OverrideName(v.vmbda.Kind, v.vmbda.Name, rules)
	v.vmbda.Spec.VirtualMachineName = common.OverrideName(v1alpha2.VirtualMachineKind, v.vmbda.Spec.VirtualMachineName, rules)

	switch v.vmbda.Spec.BlockDeviceRef.Kind {
	case v1alpha2.VMBDAObjectRefKindVirtualDisk:
		v.vmbda.Spec.BlockDeviceRef.Name = common.OverrideName(v1alpha2.VirtualDiskKind, v.vmbda.Spec.BlockDeviceRef.Name, rules)
	case v1alpha2.VMBDAObjectRefKindClusterVirtualImage:
		v.vmbda.Spec.BlockDeviceRef.Name = common.OverrideName(v1alpha2.ClusterVirtualImageKind, v.vmbda.Spec.BlockDeviceRef.Name, rules)
	case v1alpha2.VMBDAObjectRefKindVirtualImage:
		v.vmbda.Spec.BlockDeviceRef.Name = common.OverrideName(v1alpha2.VirtualImageKind, v.vmbda.Spec.BlockDeviceRef.Name, rules)
	}
}

func (v *VMBlockDeviceAttachmentHandler) Customize(prefix, suffix string) {
	v.vmbda.Spec.VirtualMachineName = common.ApplyNameCustomization(v.vmbda.Spec.VirtualMachineName, prefix, suffix)
	v.vmbda.Name = common.ApplyNameCustomization(v.vmbda.Name, prefix, suffix)

	switch v.vmbda.Spec.BlockDeviceRef.Kind {
	case v1alpha2.VMBDAObjectRefKindVirtualDisk:
		v.vmbda.Spec.BlockDeviceRef.Name = common.ApplyNameCustomization(v.vmbda.Spec.BlockDeviceRef.Name, prefix, suffix)
	case v1alpha2.VMBDAObjectRefKindClusterVirtualImage:
		// Do not apply prefix/suffix customization to ClusterVirtualImage names
	case v1alpha2.VMBDAObjectRefKindVirtualImage:
		// Do not apply prefix/suffix customization to VirtualImage names
	}
}

func (v *VMBlockDeviceAttachmentHandler) ValidateRestore(ctx context.Context) error {
	vmbdaKey := types.NamespacedName{Namespace: v.vmbda.Namespace, Name: v.vmbda.Name}
	existed, err := object.FetchObject(ctx, vmbdaKey, v.client, &v1alpha2.VirtualMachineBlockDeviceAttachment{})
	if err != nil {
		return err
	}

	if existed != nil {
		if value, ok := existed.Annotations[annotations.AnnVMOPRestore]; ok && value == v.restoreUID {
			return nil
		}

		if v.vmbda.Spec.VirtualMachineName != existed.Spec.VirtualMachineName {
			return fmt.Errorf("the virtual machine block device attachment %q %w", vmbdaKey.Name, common.ErrAlreadyInUse)
		}
	}

	return nil
}

func (v *VMBlockDeviceAttachmentHandler) ValidateClone(ctx context.Context) error {
	if err := common.ValidateResourceNameLength(v.vmbda.Name, v.vmbda.Kind); err != nil {
		return err
	}

	vmbdaKey := types.NamespacedName{Namespace: v.vmbda.Namespace, Name: v.vmbda.Name}
	existed, err := object.FetchObject(ctx, vmbdaKey, v.client, &v1alpha2.VirtualMachineBlockDeviceAttachment{})
	if err != nil {
		return err
	}

	if existed != nil {
		if value, ok := existed.Annotations[annotations.AnnVMOPRestore]; ok && value == v.restoreUID {
			return nil
		}

		if existed.Spec.VirtualMachineName != v.vmbda.Spec.VirtualMachineName {
			return fmt.Errorf("VirtualMachineBlockDeviceAttachment with name %s already exists and attached to VirtualMachine %s", v.vmbda.Name, existed.Spec.VirtualMachineName)
		}

		return fmt.Errorf("VirtualMachineBlockDeviceAttachment with name %s already exists", v.vmbda.Name)
	}

	return nil
}

func (v *VMBlockDeviceAttachmentHandler) ProcessRestore(ctx context.Context) error {
	err := v.ValidateRestore(ctx)
	if err != nil {
		return err
	}

	vmbdaKey := types.NamespacedName{Namespace: v.vmbda.Namespace, Name: v.vmbda.Name}
	vmbdaObj, err := object.FetchObject(ctx, vmbdaKey, v.client, &v1alpha2.VirtualMachineBlockDeviceAttachment{})
	if err != nil {
		return fmt.Errorf("failed to fetch the `VirtualMachineBlockDeviceAttachment`: %w", err)
	}

	if object.IsTerminating(vmbdaObj) {
		return fmt.Errorf("waiting for the `VirtualMachineBlockDeviceAttachment` %s %w", vmbdaObj.Name, common.ErrRestoring)
	}

	if vmbdaObj != nil {
		if value, ok := vmbdaObj.Annotations[annotations.AnnVMOPRestore]; ok && value == v.restoreUID {
			return nil
		}

		// Phase 1: Initiate deletion and wait for completion
		if !object.IsTerminating(vmbdaObj) {
			err = v.client.Delete(ctx, vmbdaObj)
			if err != nil {
				return fmt.Errorf("failed to delete the `VirtualMachineBlockDeviceAttachment`: %w", err)
			}
		}

		// Phase 2: Wait for deletion to complete before creating new VMBDA
		return fmt.Errorf("waiting for deletion of VirtualMachineBlockDeviceAttachment %s %w", vmbdaObj.Name, common.ErrWaitingForDeletion)
	} else {
		err = v.client.Create(ctx, v.vmbda)
		if err != nil {
			return fmt.Errorf("failed to create the `VirtualMachineBlockDeviceAttachment`: %w", err)
		}
	}

	return nil
}

func (v *VMBlockDeviceAttachmentHandler) ProcessClone(ctx context.Context) error {
	err := v.ValidateClone(ctx)
	if err != nil {
		return err
	}

	vmbdaKey := types.NamespacedName{Namespace: v.vmbda.Namespace, Name: v.vmbda.Name}
	existed, err := object.FetchObject(ctx, vmbdaKey, v.client, &v1alpha2.VirtualMachineBlockDeviceAttachment{})
	if err != nil {
		return err
	}

	if existed != nil {
		if value, ok := existed.Annotations[annotations.AnnVMOPRestore]; ok && value == v.restoreUID {
			return nil
		}
	}

	err = v.client.Create(ctx, v.vmbda)
	if err != nil {
		return fmt.Errorf("failed to create the `VirtualMachineBlockDeviceAttachment`: %w", err)
	}

	return nil
}

func (v *VMBlockDeviceAttachmentHandler) Object() client.Object {
	return v.vmbda
}
