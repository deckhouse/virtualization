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
	"github.com/deckhouse/virtualization-controller/pkg/logger"
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

func (w VirtualDiskWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	return ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.VirtualDisk{}),
		handler.EnqueueRequestsFromMapFunc(w.enqueueRequests),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				return true
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return true
			},
			UpdateFunc: w.filterUpdateEvents,
		},
	)
}

func (w VirtualDiskWatcher) enqueueRequests(ctx context.Context, obj client.Object) (requests []reconcile.Request) {
	var cviList virtv2.ClusterVirtualImageList
	err := w.client.List(ctx, &cviList)
	if err != nil {
		logger.FromContext(ctx).Error(fmt.Sprintf("failed to list cvi: %s", err))
		return
	}

	// We need to trigger reconcile for the cvi resources that use changed disk as a datasource so they can continue provisioning.
	for _, cvi := range cviList.Items {
		if cvi.Spec.DataSource.Type != virtv2.DataSourceTypeObjectRef || cvi.Spec.DataSource.ObjectRef == nil {
			continue
		}

		if cvi.Spec.DataSource.ObjectRef.Kind != virtv2.VirtualDiskKind || cvi.Spec.DataSource.ObjectRef.Name != obj.GetName() {
			continue
		}

		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name: cvi.Name,
			},
		})
	}

	vd, ok := obj.(*virtv2.VirtualDisk)
	if ok && vd.Spec.DataSource != nil && vd.Spec.DataSource.Type == virtv2.DataSourceTypeObjectRef {
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

func (w VirtualDiskWatcher) filterUpdateEvents(e event.UpdateEvent) bool {
	oldVD, ok := e.ObjectOld.(*virtv2.VirtualDisk)
	if !ok {
		return false
	}

	newVD, ok := e.ObjectNew.(*virtv2.VirtualDisk)
	if !ok {
		return false
	}

	oldInUseCondition, _ := conditions.GetCondition(vdcondition.InUseType, oldVD.Status.Conditions)
	newInUseCondition, _ := conditions.GetCondition(vdcondition.InUseType, newVD.Status.Conditions)

	oldReadyCondition, _ := conditions.GetCondition(vdcondition.ReadyType, oldVD.Status.Conditions)
	newReadyCondition, _ := conditions.GetCondition(vdcondition.ReadyType, newVD.Status.Conditions)

	if oldVD.Status.Phase != newVD.Status.Phase ||
		len(oldVD.Status.AttachedToVirtualMachines) != len(newVD.Status.AttachedToVirtualMachines) ||
		oldInUseCondition.Status != newInUseCondition.Status ||
		oldReadyCondition.Status != newReadyCondition.Status {
		return true
	}

	return false
}
