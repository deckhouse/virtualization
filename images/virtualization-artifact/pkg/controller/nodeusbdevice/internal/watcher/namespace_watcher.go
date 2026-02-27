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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func NewNamespaceWatcher() *NamespaceWatcher {
	return &NamespaceWatcher{}
}

type NamespaceWatcher struct{}

func (w *NamespaceWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	return ctr.Watch(
		source.Kind(mgr.GetCache(),
			&corev1.Namespace{},
			handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context, ns *corev1.Namespace) []reconcile.Request {
				return requestsByNamespace(ctx, mgr.GetClient(), ns.Name)
			}),
			predicate.TypedFuncs[*corev1.Namespace]{
				CreateFunc: func(e event.TypedCreateEvent[*corev1.Namespace]) bool {
					return true
				},
				DeleteFunc: func(e event.TypedDeleteEvent[*corev1.Namespace]) bool {
					return true
				},
				UpdateFunc: func(e event.TypedUpdateEvent[*corev1.Namespace]) bool {
					return true
				},
			},
		),
	)
}

func requestsByNamespace(ctx context.Context, cl client.Client, nsName string) []reconcile.Request {
	if nsName == "" {
		return nil
	}

	var nodeUSBDeviceList v1alpha2.NodeUSBDeviceList
	if err := cl.List(ctx, &nodeUSBDeviceList, client.MatchingFields{indexer.IndexFieldNodeUSBDeviceByAssignedNamespace: nsName}); err != nil {
		logger.FromContext(ctx).Error("failed to list NodeUSBDevices by assigned namespace", "error", err)
		return nil
	}

	var requests []reconcile.Request
	for _, nodeUSB := range nodeUSBDeviceList.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{Namespace: nodeUSB.Namespace, Name: nodeUSB.Name},
		})
	}

	return requests
}
