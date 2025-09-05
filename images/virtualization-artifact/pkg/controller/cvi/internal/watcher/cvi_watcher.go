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
	"github.com/deckhouse/virtualization/api/core/v1alpha2/cvicondition"
)

type ClusterVirtualImageWatcher struct {
	client client.Client
	logger *log.Logger
}

func NewClusterVirtualImageWatcher(client client.Client) *ClusterVirtualImageWatcher {
	return &ClusterVirtualImageWatcher{
		client: client,
		logger: log.Default().With("watcher", "cvi"),
	}
}

func (w ClusterVirtualImageWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(),
			&virtv2.ClusterVirtualImage{},
			handler.TypedEnqueueRequestsFromMapFunc(w.enqueueRequests),
			predicate.TypedFuncs[*virtv2.ClusterVirtualImage]{
				UpdateFunc: func(e event.TypedUpdateEvent[*virtv2.ClusterVirtualImage]) bool {
					if e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration() {
						return true
					}

					oldReadyCondition, _ := conditions.GetCondition(cvicondition.ReadyType, e.ObjectOld.Status.Conditions)
					newReadyCondition, _ := conditions.GetCondition(cvicondition.ReadyType, e.ObjectNew.Status.Conditions)

					if e.ObjectOld.Status.Phase != e.ObjectNew.Status.Phase || oldReadyCondition.Status != newReadyCondition.Status {
						return true
					}

					return false
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on CVIs: %w", err)
	}
	return nil
}

func (w ClusterVirtualImageWatcher) enqueueRequests(ctx context.Context, obj *virtv2.ClusterVirtualImage) (requests []reconcile.Request) {
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

		if cvi.Spec.DataSource.ObjectRef.Kind != virtv2.ClusterVirtualImageKind || cvi.Spec.DataSource.ObjectRef.Name != obj.GetName() {
			continue
		}

		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name: cvi.Name,
			},
		})
	}

	if obj.Spec.DataSource.Type == virtv2.DataSourceTypeObjectRef {
		if obj.Spec.DataSource.ObjectRef != nil && obj.Spec.DataSource.ObjectRef.Kind == virtv2.ClusterVirtualImageKind {
			// Need to trigger reconcile for update InUse condition.
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: obj.Spec.DataSource.ObjectRef.Name,
				},
			})
		}
	}

	requests = append(requests, reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name: obj.Name,
		},
	})

	return
}
