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

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type PodWatcher struct {
	logger *log.Logger
	client client.Client
}

func NewPodWatcher(client client.Client) *PodWatcher {
	return &PodWatcher{
		logger: log.Default().With("watcher", strings.ToLower("Pod")),
		client: client,
	}
}

func (w PodWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &corev1.Pod{},
			handler.TypedEnqueueRequestsFromMapFunc(w.enqueueRequestsFromPVC),
			predicate.TypedFuncs[*corev1.Pod]{
				UpdateFunc: func(e event.TypedUpdateEvent[*corev1.Pod]) bool {
					return e.ObjectOld.Status.Phase != e.ObjectNew.Status.Phase
				},
				CreateFunc: func(e event.TypedCreateEvent[*corev1.Pod]) bool { return false },
				DeleteFunc: func(e event.TypedDeleteEvent[*corev1.Pod]) bool { return false },
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on Pod: %w", err)
	}
	return nil
}

func (w PodWatcher) enqueueRequestsFromPVC(ctx context.Context, pod *corev1.Pod) []reconcile.Request {
	for _, ownerRef := range pod.OwnerReferences {
		if ownerRef.Kind == v1alpha2.VirtualDiskKind {
			return []reconcile.Request{{
				NamespacedName: types.NamespacedName{Name: ownerRef.Name, Namespace: pod.Namespace},
			}}
		}

		if ownerRef.Kind != "PersistentVolumeClaim" {
			continue
		}

		target, err := object.FetchObject(ctx, types.NamespacedName{Name: ownerRef.Name, Namespace: pod.Namespace}, w.client, &corev1.PersistentVolumeClaim{})
		if err != nil {
			w.logger.Error(fmt.Sprintf("failed to fetch pod owner pvc: %s", err))
			continue
		}
		if target == nil {
			continue
		}

		for _, pvcOwnerRef := range target.OwnerReferences {
			if pvcOwnerRef.Kind == v1alpha2.VirtualDiskKind {
				return []reconcile.Request{{
					NamespacedName: types.NamespacedName{Name: pvcOwnerRef.Name, Namespace: target.Namespace},
				}}
			}
		}
	}
	return nil
}
