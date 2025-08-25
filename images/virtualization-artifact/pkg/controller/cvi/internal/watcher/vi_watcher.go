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

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type VirtualImageWatcher struct {
	client client.Client
	logger *log.Logger
}

func NewVirtualImageWatcher(client client.Client) *VirtualImageWatcher {
	return &VirtualImageWatcher{
		client: client,
		logger: log.Default().With("watcher", "vi"),
	}
}

func (w VirtualImageWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(),
			&virtv2.VirtualImage{},
			handler.TypedEnqueueRequestsFromMapFunc(w.enqueueRequests),
			predicate.TypedFuncs[*virtv2.VirtualImage]{
				UpdateFunc: func(e event.TypedUpdateEvent[*virtv2.VirtualImage]) bool {
					oldReadyCondition, _ := conditions.GetCondition(vicondition.ReadyType, e.ObjectOld.Status.Conditions)
					newReadyCondition, _ := conditions.GetCondition(vicondition.ReadyType, e.ObjectNew.Status.Conditions)

					if e.ObjectOld.Status.Phase != e.ObjectNew.Status.Phase || oldReadyCondition.Status != newReadyCondition.Status {
						return true
					}

					return false
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VIs: %w", err)
	}
	return nil
}

func (w VirtualImageWatcher) enqueueRequests(ctx context.Context, vi *virtv2.VirtualImage) (requests []reconcile.Request) {
	var cviList virtv2.ClusterVirtualImageList
	err := w.client.List(ctx, &cviList)
	if err != nil {
		w.logger.Error(fmt.Sprintf("failed to list cvi: %s", err))
		return
	}

	// We need to trigger reconcile for the cvi resources that use changed image as a datasource so they can continue provisioning.
	for _, cvi := range cviList.Items {
		if cvi.Spec.DataSource.Type != virtv2.DataSourceTypeObjectRef || cvi.Spec.DataSource.ObjectRef == nil {
			continue
		}

		if cvi.Spec.DataSource.ObjectRef.Kind != virtv2.VirtualImageKind || cvi.Spec.DataSource.ObjectRef.Name != vi.GetName() {
			continue
		}

		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name: cvi.Name,
			},
		})
	}

	if vi.Spec.DataSource.Type == virtv2.DataSourceTypeObjectRef {
		if vi.Spec.DataSource.ObjectRef != nil && vi.Spec.DataSource.ObjectRef.Kind == virtv2.ClusterVirtualImageKind {
			// Need to trigger reconcile for update InUse condition.
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: vi.Spec.DataSource.ObjectRef.Name,
				},
			})
		}
	}

	return
}
