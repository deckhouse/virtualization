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
	"reflect"

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

func (v *PVNodeAffinityValidator) ValidateCreate(_ context.Context, _ *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	return nil, nil
}

func (v *PVNodeAffinityValidator) ValidateUpdate(ctx context.Context, oldVM, newVM *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	if reflect.DeepEqual(oldVM.Spec.BlockDeviceRefs, newVM.Spec.BlockDeviceRefs) {
		return nil, nil
	}

	if newVM.Status.Node == "" {
		return nil, nil
	}

	kvvmi, err := v.attacher.GetKVVMI(ctx, newVM)
	if err != nil {
		return nil, fmt.Errorf("failed to get KVVMI for VM %q: %w", newVM.Name, err)
	}
	if kvvmi == nil {
		return nil, nil
	}

	oldRefs := make(map[string]struct{}, len(oldVM.Spec.BlockDeviceRefs))
	for _, ref := range oldVM.Spec.BlockDeviceRefs {
		key := string(ref.Kind) + "/" + ref.Name
		oldRefs[key] = struct{}{}
	}

	for _, ref := range newVM.Spec.BlockDeviceRefs {
		key := string(ref.Kind) + "/" + ref.Name
		if _, existed := oldRefs[key]; existed {
			continue
		}

		ad, err := v.resolveAttachmentDisk(ctx, ref, newVM.Namespace)
		if err != nil {
			return nil, err
		}
		if ad == nil || ad.PVCName == "" {
			continue
		}

		pvc, err := v.attacher.GetPersistentVolumeClaim(ctx, ad)
		if err != nil {
			return nil, fmt.Errorf("failed to get PVC %q: %w", ad.PVCName, err)
		}
		if pvc == nil {
			continue
		}

		available, err := v.attacher.IsPVAvailableOnVMNode(ctx, pvc, kvvmi)
		if err != nil {
			return nil, fmt.Errorf("failed to check PV availability: %w", err)
		}
		if !available {
			return nil, fmt.Errorf(
				"unable to attach disk %q to VM %q: the disk is not available on node %q where the VM is running",
				ref.Name, newVM.Name, newVM.Status.Node,
			)
		}
	}

	return nil, nil
}

func (v *PVNodeAffinityValidator) resolveAttachmentDisk(ctx context.Context, ref v1alpha2.BlockDeviceSpecRef, namespace string) (*service.AttachmentDisk, error) {
	switch ref.Kind {
	case v1alpha2.DiskDevice:
		vd, err := v.attacher.GetVirtualDisk(ctx, ref.Name, namespace)
		if err != nil {
			return nil, fmt.Errorf("failed to get VirtualDisk %q: %w", ref.Name, err)
		}
		if vd == nil {
			return nil, nil
		}
		return service.NewAttachmentDiskFromVirtualDisk(vd), nil
	case v1alpha2.ImageDevice:
		vi, err := v.attacher.GetVirtualImage(ctx, ref.Name, namespace)
		if err != nil {
			return nil, fmt.Errorf("failed to get VirtualImage %q: %w", ref.Name, err)
		}
		if vi == nil {
			return nil, nil
		}
		return service.NewAttachmentDiskFromVirtualImage(vi), nil
	case v1alpha2.ClusterImageDevice:
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
