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

package internal

import (
	"context"
	"slices"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization-controller/pkg/common/network/sdnapi"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmipcondition"
)

type ProtectionHandler struct {
	client client.Client
}

func NewProtectionHandler(client client.Client) *ProtectionHandler {
	return &ProtectionHandler{client: client}
}

func (h *ProtectionHandler) Handle(ctx context.Context, vmip *v1alpha2.VirtualMachineIPAddress) (reconcile.Result, error) {
	controllerutil.AddFinalizer(vmip, v1alpha2.FinalizerIPAddressCleanup)

	// 1. The vmip has a finalizer throughout its lifetime to prevent it from being deleted without prior processing by the controller.
	if vmip.GetDeletionTimestamp() == nil {
		return reconcile.Result{}, nil
	}

	// 2. It is necessary to keep vmip protected until we can unequivocally ensure that the resource is not in the Attached state.
	attachedCondition, _ := conditions.GetCondition(vmipcondition.AttachedType, vmip.Status.Conditions)
	if attachedCondition.Status == metav1.ConditionTrue || !conditions.IsLastUpdated(attachedCondition, vmip) {
		return reconcile.Result{}, nil
	}

	// 3. The mirror must not outlive the vmip unused, nor go with it while a machine names it.
	if err := h.cleanupUnusedMirror(ctx, vmip); err != nil {
		return reconcile.Result{}, err
	}

	// 4. All checks have passed, the resource can be deleted.
	controllerutil.RemoveFinalizer(vmip, v1alpha2.FinalizerIPAddressCleanup)

	// 5. Remove legacy finalizer as well. It no longer attaches to new resources, but must be removed from old ones.
	controllerutil.RemoveFinalizer(vmip, v1alpha2.FinalizerIPAddressProtection)

	return reconcile.Result{}, nil
}

// cleanupUnusedMirror deletes the mirror of the vmip unless a machine uses it. A
// mirror a machine names in its spec loses its ownerRef on the vmip instead: GC
// would delete it together with the vmip, from under the machine.
func (h *ProtectionHandler) cleanupUnusedMirror(ctx context.Context, vmip *v1alpha2.VirtualMachineIPAddress) error {
	mirror, err := network.GetMirrorOfVMIP(ctx, h.client, vmip)
	if err != nil || mirror == nil {
		return err
	}
	if mirrorInUse(mirror) {
		return nil
	}
	ipAddressOfVM, err := h.isIPAddressOfVM(ctx, vmip.Namespace, mirror.GetName())
	if err != nil {
		return err
	}
	if ipAddressOfVM {
		return h.dropOwnerRefOnVMIP(ctx, mirror, vmip)
	}

	return client.IgnoreNotFound(h.client.Delete(ctx, mirror))
}

func (h *ProtectionHandler) dropOwnerRefOnVMIP(ctx context.Context, mirror *sdnapi.IPAddress, vmip *v1alpha2.VirtualMachineIPAddress) error {
	ownerRefsWithoutVMIP := slices.DeleteFunc(slices.Clone(mirror.GetOwnerReferences()), func(ref metav1.OwnerReference) bool {
		return ref.UID == vmip.UID
	})
	if len(ownerRefsWithoutVMIP) == len(mirror.GetOwnerReferences()) {
		return nil
	}
	ipaWithoutVMIPOwner := mirror.DeepCopy()
	ipaWithoutVMIPOwner.SetOwnerReferences(ownerRefsWithoutVMIP)
	return client.IgnoreNotFound(h.client.Patch(ctx, ipaWithoutVMIPOwner, client.MergeFrom(mirror)))
}

// mirrorInUse reports whether a pod runs on the mirror or a machine finds it by its
// label for the next pod. An ownerRef left on the mirror of a previous vmip does not
// count: the machine no longer uses it once its pod is gone.
func mirrorInUse(mirror *sdnapi.IPAddress) bool {
	return len(mirror.Status.UsedByPods) > 0 || mirror.GetLabels()[annotations.LabelVirtualMachineUID] != ""
}

func (h *ProtectionHandler) isIPAddressOfVM(ctx context.Context, namespace, mirrorName string) (bool, error) {
	var vms v1alpha2.VirtualMachineList
	if err := h.client.List(ctx, &vms, client.InNamespace(namespace)); err != nil {
		return false, err
	}
	for i := range vms.Items {
		for _, netSpec := range vms.Items[i].Spec.Networks {
			if netSpec.IPAddressName == mirrorName {
				return true, nil
			}
		}
	}
	return false, nil
}
