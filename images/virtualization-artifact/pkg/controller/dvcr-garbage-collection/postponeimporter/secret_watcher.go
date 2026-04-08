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

package postponeimporter

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/deckhouse/deckhouse/pkg/log"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/dvcr-garbage-collection/internal/watcher"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/cvicondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type DVCRGarbageCollectionSecretWatcher[object client.Object] struct {
	client client.Client
	logger *log.Logger
}

func NewWatcher[T client.Object](client client.Client, logger *log.Logger) *DVCRGarbageCollectionSecretWatcher[T] {
	if logger == nil {
		logger = log.Default()
	}
	return &DVCRGarbageCollectionSecretWatcher[T]{
		client: client,
		logger: logger.With("watcher", strings.ToLower("Secret")),
	}
}

func (w *DVCRGarbageCollectionSecretWatcher[T]) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(
			mgr.GetCache(),
			&corev1.Secret{},
			handler.TypedEnqueueRequestsFromMapFunc(w.enqueuePostponedResources),
			predicate.TypedFuncs[*corev1.Secret]{
				CreateFunc: func(e event.TypedCreateEvent[*corev1.Secret]) bool {
					return watcher.IsDVCRGarbageCollectionSecret(e.Object)
				},
				UpdateFunc: func(e event.TypedUpdateEvent[*corev1.Secret]) bool {
					if !watcher.IsDVCRGarbageCollectionSecret(e.ObjectNew) {
						return false
					}
					return !reflect.DeepEqual(e.ObjectNew.GetAnnotations(), e.ObjectOld.GetAnnotations())
				},
				DeleteFunc: func(e event.TypedDeleteEvent[*corev1.Secret]) bool {
					return watcher.IsDVCRGarbageCollectionSecret(e.Object)
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on DVCR garbage collection Secret: %w", err)
	}
	return nil
}

func (w *DVCRGarbageCollectionSecretWatcher[T]) enqueuePostponedResources(ctx context.Context, _ *corev1.Secret) []reconcile.Request {
	var resList client.ObjectList

	// Type switch for T should be done on an empty value for T.
	var obj T
	switch any(obj).(type) {
	case *v1alpha2.ClusterVirtualImage:
		resList = &v1alpha2.ClusterVirtualImageList{}
	case *v1alpha2.VirtualImage:
		resList = &v1alpha2.VirtualImageList{}
	case *v1alpha2.VirtualDisk:
		resList = &v1alpha2.VirtualDiskList{}
	default:
		return nil
	}

	if err := w.client.List(ctx, resList, &client.ListOptions{}); err != nil {
		w.logger.Error("list %T resources: %v", obj, err)
		return nil
	}

	requests := make([]reconcile.Request, 0)
	switch obj := resList.(type) {
	case *v1alpha2.ClusterVirtualImageList:
		for _, item := range obj.Items {
			if isClusterVirtualImagePostponed(&item) {
				requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&item)})
			}
		}
	case *v1alpha2.VirtualDiskList:
		for _, item := range obj.Items {
			if isVirtualDiskPostponed(&item) {
				requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&item)})
			}
		}
	case *v1alpha2.VirtualImageList:
		for _, item := range obj.Items {
			if isVirtualImagePostponed(&item) {
				requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&item)})
			}
		}
	}

	return requests
}

func isClusterVirtualImagePostponed(cvi *v1alpha2.ClusterVirtualImage) bool {
	cond, ok := conditions.GetCondition(cvicondition.ReadyType, cvi.Status.Conditions)
	return ok && cond.Reason == ProvisioningPostponedReason.String()
}

func isVirtualImagePostponed(vi *v1alpha2.VirtualImage) bool {
	cond, ok := conditions.GetCondition(vicondition.ReadyType, vi.Status.Conditions)
	return ok && cond.Reason == ProvisioningPostponedReason.String()
}

func isVirtualDiskPostponed(vd *v1alpha2.VirtualDisk) bool {
	cond, ok := conditions.GetCondition(vdcondition.ReadyType, vd.Status.Conditions)
	return ok && cond.Reason == ProvisioningPostponedReason.String()
}
