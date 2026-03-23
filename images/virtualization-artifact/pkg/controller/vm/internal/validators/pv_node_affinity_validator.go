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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/component-helpers/scheduling/corev1/nodeaffinity"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type PVNodeAffinityValidator struct {
	client client.Client
}

func NewPVNodeAffinityValidator(client client.Client) *PVNodeAffinityValidator {
	return &PVNodeAffinityValidator{client: client}
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

	oldRefs := make(map[string]struct{}, len(oldVM.Spec.BlockDeviceRefs))
	for _, ref := range oldVM.Spec.BlockDeviceRefs {
		key := string(ref.Kind) + "/" + ref.Name
		oldRefs[key] = struct{}{}
	}

	node := &corev1.Node{}
	if err := v.client.Get(ctx, types.NamespacedName{Name: newVM.Status.Node}, node); err != nil {
		return nil, fmt.Errorf("failed to get node %q: %w", newVM.Status.Node, err)
	}

	for _, ref := range newVM.Spec.BlockDeviceRefs {
		key := string(ref.Kind) + "/" + ref.Name
		if _, existed := oldRefs[key]; existed {
			continue
		}

		available, err := v.isDiskAvailableOnNode(ctx, ref, newVM.Namespace, node)
		if err != nil {
			return nil, err
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

func (v *PVNodeAffinityValidator) isDiskAvailableOnNode(ctx context.Context, ref v1alpha2.BlockDeviceSpecRef, namespace string, node *corev1.Node) (bool, error) {
	var pvcName string

	switch ref.Kind {
	case v1alpha2.DiskDevice:
		vd, err := object.FetchObject(ctx, types.NamespacedName{Name: ref.Name, Namespace: namespace}, v.client, &v1alpha2.VirtualDisk{})
		if err != nil {
			return false, fmt.Errorf("failed to get VirtualDisk %q: %w", ref.Name, err)
		}
		if vd == nil {
			return true, nil
		}
		pvcName = vd.Status.Target.PersistentVolumeClaim
	case v1alpha2.ImageDevice:
		vi, err := object.FetchObject(ctx, types.NamespacedName{Name: ref.Name, Namespace: namespace}, v.client, &v1alpha2.VirtualImage{})
		if err != nil {
			return false, fmt.Errorf("failed to get VirtualImage %q: %w", ref.Name, err)
		}
		if vi == nil {
			return true, nil
		}
		if vi.Spec.Storage == v1alpha2.StorageContainerRegistry {
			return true, nil
		}
		pvcName = vi.Status.Target.PersistentVolumeClaim
	case v1alpha2.ClusterImageDevice:
		return true, nil
	default:
		return true, nil
	}

	if pvcName == "" {
		return true, nil
	}

	pvc, err := object.FetchObject(ctx, types.NamespacedName{Name: pvcName, Namespace: namespace}, v.client, &corev1.PersistentVolumeClaim{})
	if err != nil {
		return false, fmt.Errorf("failed to get PVC %q: %w", pvcName, err)
	}
	if pvc == nil || pvc.Spec.VolumeName == "" {
		return true, nil
	}

	pv, err := object.FetchObject(ctx, types.NamespacedName{Name: pvc.Spec.VolumeName}, v.client, &corev1.PersistentVolume{})
	if err != nil {
		return false, fmt.Errorf("failed to get PV %q: %w", pvc.Spec.VolumeName, err)
	}
	if pv == nil || pv.Spec.NodeAffinity == nil || pv.Spec.NodeAffinity.Required == nil {
		return true, nil
	}

	selector, err := nodeaffinity.NewNodeSelector(pv.Spec.NodeAffinity.Required)
	if err != nil {
		return false, fmt.Errorf("failed to parse PV node selector: %w", err)
	}

	return selector.Match(node), nil
}
