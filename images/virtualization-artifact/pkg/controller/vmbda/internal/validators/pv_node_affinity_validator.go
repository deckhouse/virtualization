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

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	k8snodeaffinity "k8s.io/component-helpers/scheduling/corev1/nodeaffinity"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/common/nodeaffinity"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type PVNodeAffinityValidator struct {
	client   client.Client
	attacher *service.AttachmentService
}

func NewPVNodeAffinityValidator(client client.Client, attacher *service.AttachmentService) *PVNodeAffinityValidator {
	return &PVNodeAffinityValidator{client: client, attacher: attacher}
}

func (v *PVNodeAffinityValidator) ValidateCreate(ctx context.Context, vmbda *v1alpha2.VirtualMachineBlockDeviceAttachment) (admission.Warnings, error) {
	vm, err := v.attacher.GetVirtualMachine(ctx, vmbda.Spec.VirtualMachineName, vmbda.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get VirtualMachine %q: %w", vmbda.Spec.VirtualMachineName, err)
	}
	if vm == nil {
		return nil, nil
	}

	if vm.Status.Node != "" {
		return v.validateScheduledVM(ctx, vm, vmbda)
	}

	return v.validateUnscheduledVM(ctx, vm, vmbda)
}

func (v *PVNodeAffinityValidator) ValidateUpdate(_ context.Context, _, _ *v1alpha2.VirtualMachineBlockDeviceAttachment) (admission.Warnings, error) {
	return nil, nil
}

func (v *PVNodeAffinityValidator) validateScheduledVM(ctx context.Context, vm *v1alpha2.VirtualMachine, vmbda *v1alpha2.VirtualMachineBlockDeviceAttachment) (admission.Warnings, error) {
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
			`unable to attach disk to VM %q: the disk %q is not available on node %q where the VM is running`,
			vmbda.Spec.VirtualMachineName, vmbda.Spec.BlockDeviceRef.Name, vm.Status.Node,
		)
	}

	return nil, nil
}

func (v *PVNodeAffinityValidator) validateUnscheduledVM(ctx context.Context, vm *v1alpha2.VirtualMachine, vmbda *v1alpha2.VirtualMachineBlockDeviceAttachment) (admission.Warnings, error) {
	log := logger.FromContext(ctx)
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
	if pvc == nil || pvc.Spec.VolumeName == "" {
		return nil, nil
	}

	var pv corev1.PersistentVolume
	if err := v.client.Get(ctx, types.NamespacedName{Name: pvc.Spec.VolumeName}, &pv); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get PersistentVolume %q: %w", pvc.Spec.VolumeName, err)
	}
	if pv.Spec.NodeAffinity == nil || pv.Spec.NodeAffinity.Required == nil {
		log.Info("PersistentVolume has no node affinity, no topology constraint applied", "pv", pv.Name, "pvc", ad.PVCName)
		return nil, nil
	}

	pvSel, err := k8snodeaffinity.NewNodeSelector(pv.Spec.NodeAffinity.Required)
	if err != nil {
		return nil, fmt.Errorf("build node selector for PV %q: %w", pv.Name, err)
	}

	var vmClass v1alpha2.VirtualMachineClass
	if err := v.client.Get(ctx, types.NamespacedName{Name: vm.Spec.VirtualMachineClassName}, &vmClass); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get VirtualMachineClass %q: %w", vm.Spec.VirtualMachineClassName, err)
	}

	var nodeList corev1.NodeList
	if err := v.client.List(ctx, &nodeList); err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	for i := range nodeList.Items {
		node := &nodeList.Items[i]
		match, err := nodeaffinity.MatchesVMPlacement(node, vm, &vmClass)
		if err != nil {
			return nil, fmt.Errorf("match VM placement for node %q: %w", node.Name, err)
		}
		if match && pvSel.Match(node) {
			return nil, nil
		}
	}

	return nil, fmt.Errorf(
		`unable to attach disk to VM %q due to a topology conflict. Ensure that disk %q is accessible on the nodes in accordance with the VM node placement rules (node selector, affinity, tolerations)`,
		vmbda.Spec.VirtualMachineName, vmbda.Spec.BlockDeviceRef.Name,
	)
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
