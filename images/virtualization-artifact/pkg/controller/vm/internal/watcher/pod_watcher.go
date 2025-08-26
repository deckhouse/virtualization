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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
)

func NewPodWatcher() *PodWatcher {
	return &PodWatcher{}
}

type PodWatcher struct{}

func (w *PodWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	// Watch for Pods created on behalf of VMs. Handle only changes in status.phase.
	// Pod tracking is required to detect when Pod becomes Completed after guest initiated reset or shutdown.
	if err := ctr.Watch(
		source.Kind(
			mgr.GetCache(),
			&corev1.Pod{},
			handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context, pod *corev1.Pod) []reconcile.Request {
				vmName, hasLabel := pod.GetLabels()["vm.kubevirt.io/name"]
				if !hasLabel {
					return nil
				}

				return []reconcile.Request{
					{
						NamespacedName: types.NamespacedName{
							Name:      vmName,
							Namespace: pod.GetNamespace(),
						},
					},
				}
			}),
			predicate.TypedFuncs[*corev1.Pod]{
				CreateFunc: func(e event.TypedCreateEvent[*corev1.Pod]) bool { return true },
				DeleteFunc: func(e event.TypedDeleteEvent[*corev1.Pod]) bool { return true },
				UpdateFunc: func(e event.TypedUpdateEvent[*corev1.Pod]) bool {
					return e.ObjectOld.Status.Phase != e.ObjectNew.Status.Phase ||
						e.ObjectOld.Annotations[annotations.AnnNetworksStatus] != e.ObjectNew.Annotations[annotations.AnnNetworksStatus]
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on Pod: %w", err)
	}
	return nil
}
