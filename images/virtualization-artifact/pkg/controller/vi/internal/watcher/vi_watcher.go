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
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type VirtualImageWatcher struct {
	client client.Client
}

func NewVirtualImageWatcher(client client.Client) *VirtualImageWatcher {
	return &VirtualImageWatcher{
		client: client,
	}
}

func (w VirtualImageWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	return ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.VirtualImage{}),
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

func (w VirtualImageWatcher) enqueueRequests(ctx context.Context, obj client.Object) (requests []reconcile.Request) {
	var viList virtv2.VirtualImageList
	err := w.client.List(ctx, &viList, &client.ListOptions{
		Namespace: obj.GetNamespace(),
	})
	if err != nil {
		logger.FromContext(ctx).Error(fmt.Sprintf("failed to list vi: %s", err))
		return
	}

	for _, vi := range viList.Items {
		if vi.Spec.DataSource.Type != virtv2.DataSourceTypeObjectRef || vi.Spec.DataSource.ObjectRef == nil {
			continue
		}

		if vi.Spec.DataSource.ObjectRef.Kind != virtv2.VirtualImageKind || vi.Spec.DataSource.ObjectRef.Name != obj.GetName() {
			continue
		}

		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      vi.Name,
				Namespace: vi.Namespace,
			},
		})
	}

	return
}

func (w VirtualImageWatcher) filterUpdateEvents(e event.UpdateEvent) bool {
	oldVI, ok := e.ObjectOld.(*virtv2.VirtualImage)
	if !ok {
		return false
	}

	newVI, ok := e.ObjectNew.(*virtv2.VirtualImage)
	if !ok {
		return false
	}

	oldReadyCondition, _ := conditions.GetCondition(vicondition.ReadyType, oldVI.Status.Conditions)
	newReadyCondition, _ := conditions.GetCondition(vicondition.ReadyType, newVI.Status.Conditions)

	if oldVI.Status.Phase != newVI.Status.Phase || oldReadyCondition.Status != newReadyCondition.Status {
		return true
	}

	return false
}
