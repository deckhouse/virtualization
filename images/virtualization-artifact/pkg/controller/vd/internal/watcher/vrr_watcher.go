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

package watcher

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/deckhouse/pkg/log"
	storagefoundationv1alpha1 "github.com/deckhouse/storage-foundation/api/v1alpha1"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// VolumeRestoreRequestWatcher wakes the owning VirtualDisk when its VolumeRestoreRequest changes.
//
// This exists because the PVC that the patched CSI external-provisioner creates for a
// VolumeRestoreRequest is NOT owned by the VirtualDisk (its ownerReference points at the VRR's own
// ObjectKeeper) — so PersistentVolumeClaimWatcher's ownerRef-based enqueue never fires when that PVC
// becomes Bound, and the VirtualDisk would otherwise sit in Provisioning until some unrelated event (or
// the informer resync) happens to trigger a reconcile. The VolumeRestoreRequest itself IS created with
// an ownerReference to the VirtualDisk (see PersistentVolumeClaimService.EnsureVolumeRestoreRequest), so
// watching it directly closes that gap.
type VolumeRestoreRequestWatcher struct {
	logger *log.Logger
}

func NewVolumeRestoreRequestWatcher() *VolumeRestoreRequestWatcher {
	return &VolumeRestoreRequestWatcher{
		logger: log.Default().With("watcher", strings.ToLower("VolumeRestoreRequest")),
	}
}

func (w VolumeRestoreRequestWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &storagefoundationv1alpha1.VolumeRestoreRequest{},
			handler.TypedEnqueueRequestsFromMapFunc(func(_ context.Context, vrr *storagefoundationv1alpha1.VolumeRestoreRequest) []reconcile.Request {
				return w.enqueueRequestsFromOwnerRefs(vrr)
			}),
			predicate.TypedFuncs[*storagefoundationv1alpha1.VolumeRestoreRequest]{
				UpdateFunc: w.filterUpdateEvents,
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VolumeRestoreRequest: %w", err)
	}
	return nil
}

func (w VolumeRestoreRequestWatcher) enqueueRequestsFromOwnerRefs(obj client.Object) (requests []reconcile.Request) {
	for _, ownerRef := range obj.GetOwnerReferences() {
		if ownerRef.Kind == v1alpha2.VirtualDiskKind {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      ownerRef.Name,
					Namespace: obj.GetNamespace(),
				},
			})
		}
	}
	return requests
}

func (w VolumeRestoreRequestWatcher) filterUpdateEvents(e event.TypedUpdateEvent[*storagefoundationv1alpha1.VolumeRestoreRequest]) bool {
	if (e.ObjectOld.Status.PvcRef != nil) != (e.ObjectNew.Status.PvcRef != nil) {
		return true
	}
	return conditionsChanged(e.ObjectOld.Status.Conditions, e.ObjectNew.Status.Conditions)
}

func conditionsChanged(oldConds, newConds []metav1.Condition) bool {
	if len(oldConds) != len(newConds) {
		return true
	}

	was := make(map[string]metav1.Condition, len(oldConds))
	for _, c := range oldConds {
		was[c.Type] = c
	}
	for _, c := range newConds {
		prev, ok := was[c.Type]
		if !ok || prev.Status != c.Status || prev.Reason != c.Reason || prev.Message != c.Message {
			return true
		}
	}
	return false
}
