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
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

func NewVolumeEventWatcher(client client.Client) *VolumeEventWatcher {
	return &VolumeEventWatcher{
		client: client,
	}
}

type VolumeEventWatcher struct {
	client client.Client
}

func (w *VolumeEventWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(
			mgr.GetCache(),
			&corev1.Event{},
			handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context, e *corev1.Event) []reconcile.Request {
				if e.InvolvedObject.Kind != "Pod" {
					return nil
				}

				if e.Reason != "FailedAttachVolume" && e.Reason != "FailedMount" {
					return nil
				}

				pod := &corev1.Pod{}
				if err := w.client.Get(ctx, types.NamespacedName{
					Name:      e.InvolvedObject.Name,
					Namespace: e.InvolvedObject.Namespace,
				}, pod); err != nil {
					return nil
				}

				vmName, hasLabel := pod.GetLabels()[virtv1.VirtualMachineNameLabel]
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
			predicate.TypedFuncs[*corev1.Event]{
				CreateFunc: func(e event.TypedCreateEvent[*corev1.Event]) bool {
					return e.Object.Type == corev1.EventTypeWarning &&
						(e.Object.Reason == "FailedAttachVolume" || e.Object.Reason == "FailedMount")
				},
				UpdateFunc: func(e event.TypedUpdateEvent[*corev1.Event]) bool {
					return false
				},
				DeleteFunc: func(e event.TypedDeleteEvent[*corev1.Event]) bool {
					return false
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on Event: %w", err)
	}
	return nil
}
