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
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
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
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &v1alpha2.VirtualDisk{},
			handler.TypedEnqueueRequestsFromMapFunc(w.enqueueRequests),
			predicate.TypedFuncs[*v1alpha2.VirtualDisk]{
				CreateFunc: func(e event.TypedCreateEvent[*v1alpha2.VirtualDisk]) bool { return false },
				UpdateFunc: func(e event.TypedUpdateEvent[*v1alpha2.VirtualDisk]) bool {
					if e.ObjectOld.Status.Phase != e.ObjectNew.Status.Phase {
						return true
					}

					oldReadyCondition, _ := conditions.GetCondition(vdcondition.ReadyType, e.ObjectOld.Status.Conditions)
					newReadyCondition, _ := conditions.GetCondition(vdcondition.ReadyType, e.ObjectNew.Status.Conditions)

					return oldReadyCondition.Status != newReadyCondition.Status
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VDs: %w", err)
	}
	return nil
}

func (w VirtualDiskWatcher) enqueueRequests(ctx context.Context, vd *v1alpha2.VirtualDisk) (requests []reconcile.Request) {
	var vmbdas v1alpha2.VirtualMachineBlockDeviceAttachmentList
	err := w.client.List(ctx, &vmbdas, &client.ListOptions{
		Namespace: vd.GetNamespace(),
	})
	if err != nil {
		slog.Default().Error(fmt.Sprintf("failed to list vmbdas: %s", err))
		return
	}

	for _, vmbda := range vmbdas.Items {
		if vmbda.Spec.BlockDeviceRef.Kind != v1alpha2.VMBDAObjectRefKindVirtualDisk && vmbda.Spec.BlockDeviceRef.Name != vd.GetName() {
			continue
		}

		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      vmbda.Name,
				Namespace: vmbda.Namespace,
			},
		})
	}

	return
}
