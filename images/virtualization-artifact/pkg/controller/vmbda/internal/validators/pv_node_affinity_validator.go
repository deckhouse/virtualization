/*
Copyright 2026 Flant JSC

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

package validators

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type PVNodeAffinityValidator struct {
	attacher *service.AttachmentService
}

func NewPVNodeAffinityValidator(attacher *service.AttachmentService) *PVNodeAffinityValidator {
	return &PVNodeAffinityValidator{attacher: attacher}
}

func (v *PVNodeAffinityValidator) ValidateCreate(ctx context.Context, vmbda *v1alpha2.VirtualMachineBlockDeviceAttachment) (admission.Warnings, error) {
	vm, err := v.attacher.GetVirtualMachine(ctx, vmbda.Spec.VirtualMachineName, vmbda.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get VirtualMachine %q: %w", vmbda.Spec.VirtualMachineName, err)
	}
	if vm == nil || vm.Status.Node == "" {
		return nil, nil
	}

	kvvmi, err := v.attacher.GetKVVMI(ctx, vm)
	if err != nil {
		return nil, fmt.Errorf("failed to get KVVMI for VM %q: %w", vm.Name, err)
	}
	if kvvmi == nil {
		return nil, nil
	}

	ad, err := v.resolveAttachmentDisk(ctx, vmbda)
	if err != nil {
		return nil, err
	}
	if ad == nil || ad.PVCName == "" {
		return nil, nil
	}

	pvc, err := v.attacher.GetPersistentVolumeClaim(ctx, ad)
	if err != nil {
		return nil, fmt.Errorf("failed to get PVC %q: %w", ad.PVCName, err)
	}
	if pvc == nil {
		return nil, nil
	}

	available, err := v.attacher.IsPVAvailableOnVMNode(ctx, pvc, kvvmi)
	if err != nil {
		return nil, fmt.Errorf("failed to check PV availability: %w", err)
	}

	if !available {
		return nil, fmt.Errorf(
			"unable to attach disk %q to VM %q: the disk is not available on node %q where the VM is running",
			vmbda.Spec.BlockDeviceRef.Name, vmbda.Spec.VirtualMachineName, vm.Status.Node,
		)
	}

	return nil, nil
}

func (v *PVNodeAffinityValidator) ValidateUpdate(_ context.Context, _, _ *v1alpha2.VirtualMachineBlockDeviceAttachment) (admission.Warnings, error) {
	return nil, nil
}

func (v *PVNodeAffinityValidator) resolveAttachmentDisk(ctx context.Context, vmbda *v1alpha2.VirtualMachineBlockDeviceAttachment) (*service.AttachmentDisk, error) {
	ref := vmbda.Spec.BlockDeviceRef

	switch ref.Kind {
	case v1alpha2.VMBDAObjectRefKindVirtualDisk:
		vd, err := v.attacher.GetVirtualDisk(ctx, ref.Name, vmbda.Namespace)
		if err != nil {
			return nil, fmt.Errorf("failed to get VirtualDisk %q: %w", ref.Name, err)
		}
		if vd == nil {
			return nil, nil
		}
		return service.NewAttachmentDiskFromVirtualDisk(vd), nil
	case v1alpha2.VMBDAObjectRefKindVirtualImage:
		vi, err := v.attacher.GetVirtualImage(ctx, ref.Name, vmbda.Namespace)
		if err != nil {
			return nil, fmt.Errorf("failed to get VirtualImage %q: %w", ref.Name, err)
		}
		if vi == nil {
			return nil, nil
		}
		return service.NewAttachmentDiskFromVirtualImage(vi), nil
	case v1alpha2.VMBDAObjectRefKindClusterVirtualImage:
		cvi, err := v.attacher.GetClusterVirtualImage(ctx, ref.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to get ClusterVirtualImage %q: %w", ref.Name, err)
		}
		if cvi == nil {
			return nil, nil
		}
		return service.NewAttachmentDiskFromClusterVirtualImage(cvi), nil
	default:
		return nil, nil
	}
}
