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
	logger *log.Logger
	client client.Client
}

func NewVirtualImageWatcher(client client.Client) *VirtualImageWatcher {
	return &VirtualImageWatcher{
		logger: log.Default().With("watcher", "vi"),
		client: client,
	}
}

func (w VirtualImageWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	return ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.VirtualImage{},
			handler.TypedEnqueueRequestsFromMapFunc(w.enqueueRequests),
			predicate.TypedFuncs[*virtv2.VirtualImage]{
				UpdateFunc: func(e event.TypedUpdateEvent[*virtv2.VirtualImage]) bool {
					if e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration() {
						return true
					}

					oldReadyCondition, _ := conditions.GetCondition(vicondition.ReadyType, e.ObjectOld.Status.Conditions)
					newReadyCondition, _ := conditions.GetCondition(vicondition.ReadyType, e.ObjectNew.Status.Conditions)

					if e.ObjectOld.Status.Phase != e.ObjectNew.Status.Phase || oldReadyCondition.Status != newReadyCondition.Status {
						return true
					}

					return false
				},
			},
		),
	)
}

func (w VirtualImageWatcher) enqueueRequests(ctx context.Context, obj *virtv2.VirtualImage) (requests []reconcile.Request) {
	var viList virtv2.VirtualImageList
	err := w.client.List(ctx, &viList, &client.ListOptions{
		Namespace: obj.GetNamespace(),
	})
	if err != nil {
		w.logger.Error(fmt.Sprintf("failed to list vi: %s", err))
		return
	}

	// We need to trigger reconcile for the vi resources that use changed image as a datasource so they can continue provisioning.
	for _, vi := range viList.Items {
		if vi.Spec.DataSource.Type != virtv2.DataSourceTypeObjectRef || vi.Spec.DataSource.ObjectRef == nil {
			continue
		}

		if vi.Spec.DataSource.ObjectRef.Kind != virtv2.VirtualImageKind || vi.Spec.DataSource.ObjectRef.Name != vi.GetName() {
			continue
		}

		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      vi.Name,
				Namespace: vi.Namespace,
			},
		})
	}

	if obj.Spec.DataSource.Type == virtv2.DataSourceTypeObjectRef {
		if obj.Spec.DataSource.ObjectRef != nil && obj.Spec.DataSource.ObjectRef.Kind == virtv2.VirtualImageKind {
			// Need to trigger reconcile for update InUse condition.
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      obj.Spec.DataSource.ObjectRef.Name,
					Namespace: obj.Namespace,
				},
			})
		}
	}

	requests = append(requests, reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: obj.Namespace,
			Name:      obj.Name,
		},
	})

	return
}
