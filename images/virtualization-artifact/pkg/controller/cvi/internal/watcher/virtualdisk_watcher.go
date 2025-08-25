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

package watcher

import (
	"context"
	"fmt"
	"log/slog"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type VirtualDiskWatcher struct {
	client client.Client
}

func NewVirtualDiskWatcher(client client.Client) *VirtualDiskWatcher {
	return &VirtualDiskWatcher{
		client: client,
	}
}

func (w *VirtualDiskWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(
			mgr.GetCache(),
			&virtv2.VirtualDisk{},
			handler.TypedEnqueueRequestsFromMapFunc(w.enqueueRequestsFromVDs),
			predicate.TypedFuncs[*virtv2.VirtualDisk]{
				UpdateFunc: func(e event.TypedUpdateEvent[*virtv2.VirtualDisk]) bool {
					oldInUseCondition, _ := conditions.GetCondition(vdcondition.InUseType, e.ObjectOld.Status.Conditions)
					newInUseCondition, _ := conditions.GetCondition(vdcondition.InUseType, e.ObjectNew.Status.Conditions)

					oldReadyCondition, _ := conditions.GetCondition(vdcondition.ReadyType, e.ObjectOld.Status.Conditions)
					newReadyCondition, _ := conditions.GetCondition(vdcondition.ReadyType, e.ObjectNew.Status.Conditions)

					if e.ObjectOld.Status.Phase != e.ObjectNew.Status.Phase ||
						len(e.ObjectOld.Status.AttachedToVirtualMachines) != len(e.ObjectNew.Status.AttachedToVirtualMachines) ||
						oldInUseCondition != newInUseCondition ||
						oldReadyCondition.Status != newReadyCondition.Status {
						return true
					}

					return false
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VDs: %w", err)
	}
	return nil
}

func (w *VirtualDiskWatcher) enqueueRequestsFromVDs(ctx context.Context, vd *virtv2.VirtualDisk) (requests []reconcile.Request) {
	var cviList virtv2.ClusterVirtualImageList
	err := w.client.List(ctx, &cviList, &client.ListOptions{})
	if err != nil {
		slog.Default().Error(fmt.Sprintf("failed to list cvi: %s", err))
		return
	}

	for _, cvi := range cviList.Items {
		if cvi.Spec.DataSource.Type != virtv2.DataSourceTypeObjectRef || cvi.Spec.DataSource.ObjectRef == nil {
			continue
		}

		if cvi.Spec.DataSource.ObjectRef.Kind != virtv2.VirtualDiskKind || cvi.Spec.DataSource.ObjectRef.Name != vd.GetName() && cvi.Spec.DataSource.ObjectRef.Namespace != vd.GetNamespace() {
			continue
		}

		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name: cvi.Name,
			},
		})
	}

	if vd.Spec.DataSource != nil && vd.Spec.DataSource.Type == virtv2.DataSourceTypeObjectRef {
		if vd.Spec.DataSource.ObjectRef != nil && vd.Spec.DataSource.ObjectRef.Kind == virtv2.ClusterVirtualImageKind {
			// Need to trigger reconcile for update InUse condition.
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: vd.Spec.DataSource.ObjectRef.Name,
				},
			})
		}
	}

	return
}
