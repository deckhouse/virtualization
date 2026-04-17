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
	"strings"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	corev1 "k8s.io/api/core/v1"
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

func (v *PVNodeAffinityValidator) ValidateCreate(ctx context.Context, vm *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	return v.validateUnscheduledVM(ctx, vm, vm.Spec.BlockDeviceRefs, "create")
}

func (v *PVNodeAffinityValidator) ValidateUpdate(ctx context.Context, oldVM, newVM *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	if reflect.DeepEqual(oldVM.Spec.BlockDeviceRefs, newVM.Spec.BlockDeviceRefs) {
		return nil, nil
	}

	if newVM.Status.Node != "" {
		return v.validateScheduledVM(ctx, oldVM, newVM)
	}

	return v.validateUnscheduledVM(ctx, newVM, newVM.Spec.BlockDeviceRefs, "update")
}

func (v *PVNodeAffinityValidator) validateScheduledVM(ctx context.Context, oldVM, newVM *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	kvvmi, err := v.attacher.GetKVVMI(ctx, newVM)
	if err != nil {
		return nil, fmt.Errorf("failed to get KVVMI for VM %q: %w", newVM.Name, err)
	}
	if kvvmi == nil {
		return nil, nil
	}

	oldRefs := make(map[string]struct{}, len(oldVM.Spec.BlockDeviceRefs))
	for _, ref := range oldVM.Spec.BlockDeviceRefs {
		oldRefs[string(ref.Kind)+"/"+ref.Name] = struct{}{}
	}

	var incompatibleDisks []string
	for _, ref := range newVM.Spec.BlockDeviceRefs {
		if _, existed := oldRefs[string(ref.Kind)+"/"+ref.Name]; existed {
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
			incompatibleDisks = append(incompatibleDisks, ref.Name)
		}
	}

	if len(incompatibleDisks) > 0 {
		return nil, fmt.Errorf(
			`unable to attach disks to VM %q: disks ["%s"] are not available on node %q where the VM is running`,
			newVM.Name, strings.Join(incompatibleDisks, `", "`), newVM.Status.Node,
		)
	}

	return nil, nil
}

func (v *PVNodeAffinityValidator) validateUnscheduledVM(ctx context.Context, vm *v1alpha2.VirtualMachine, refs []v1alpha2.BlockDeviceSpecRef, action string) (admission.Warnings, error) {
	pvSelectors, err := v.collectPVNodeSelectors(ctx, refs, vm.Namespace)
	if err != nil {
		return nil, err
	}
	if len(pvSelectors) == 0 {
		return nil, nil
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
		if !match {
			continue
		}
		matchesAllPVs := true
		for _, pvSel := range pvSelectors {
			if !pvSel.Match(node) {
				matchesAllPVs = false
				break
			}
		}
		if matchesAllPVs {
			return nil, nil
		}
	}

	return nil, fmt.Errorf(
		`unable to %s VM %q due to a topology conflict. Ensure that all disks are accessible on the nodes in accordance with the VM node placement rules (node selector, affinity, tolerations)`,
		action, vm.Name,
	)
}

func (v *PVNodeAffinityValidator) collectPVNodeSelectors(ctx context.Context, refs []v1alpha2.BlockDeviceSpecRef, namespace string) ([]*k8snodeaffinity.NodeSelector, error) {
	log := logger.FromContext(ctx)
	var selectors []*k8snodeaffinity.NodeSelector
	for _, ref := range refs {
		ad, err := v.resolveAttachmentDisk(ctx, ref, namespace)
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
		if pvc == nil || pvc.Spec.VolumeName == "" {
			continue
		}

		var pv corev1.PersistentVolume
		if err := v.client.Get(ctx, types.NamespacedName{Name: pvc.Spec.VolumeName}, &pv); err != nil {
			if k8serrors.IsNotFound(err) {
				continue
			}
			return nil, fmt.Errorf("get PersistentVolume %q: %w", pvc.Spec.VolumeName, err)
		}

		if pv.Spec.NodeAffinity == nil || pv.Spec.NodeAffinity.Required == nil {
			log.Info("PersistentVolume has no node affinity, no topology constraint applied", "pv", pv.Name, "pvc", ad.PVCName)
			continue
		}

		ns, err := k8snodeaffinity.NewNodeSelector(pv.Spec.NodeAffinity.Required)
		if err != nil {
			return nil, fmt.Errorf("build node selector for PV %q: %w", pvc.Spec.VolumeName, err)
		}
		selectors = append(selectors, ns)
	}
	return selectors, nil
}

func (v *PVNodeAffinityValidator) resolveAttachmentDisk(ctx context.Context, ref v1alpha2.BlockDeviceSpecRef, namespace string) (*service.AttachmentDisk, error) {
	log := logger.FromContext(ctx)
	switch ref.Kind {
	case v1alpha2.DiskDevice:
		vd, err := v.attacher.GetVirtualDisk(ctx, ref.Name, namespace)
		if err != nil {
			return nil, fmt.Errorf("failed to get VirtualDisk %q: %w", ref.Name, err)
		}
		if vd == nil {
			log.Info("VirtualDisk not found, skipping PV node affinity check", "name", ref.Name, "namespace", namespace)
			return nil, nil
		}
		return service.NewAttachmentDiskFromVirtualDisk(vd), nil
	case v1alpha2.ImageDevice:
		vi, err := v.attacher.GetVirtualImage(ctx, ref.Name, namespace)
		if err != nil {
			return nil, fmt.Errorf("failed to get VirtualImage %q: %w", ref.Name, err)
		}
		if vi == nil {
			log.Info("VirtualImage not found, skipping PV node affinity check", "name", ref.Name, "namespace", namespace)
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
