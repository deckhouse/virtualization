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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type PodWatcher struct {
	client client.Client
}

func NewPodWatcher(client client.Client) *PodWatcher {
	return &PodWatcher{
		client: client,
	}
}

func (w PodWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(
			mgr.GetCache(),
			&corev1.Pod{},
			handler.TypedEnqueueRequestForOwner[*corev1.Pod](
				mgr.GetScheme(),
				mgr.GetRESTMapper(),
				&virtv2.VirtualImage{},
			),
			predicate.TypedFuncs[*corev1.Pod]{
				DeleteFunc: func(e event.TypedDeleteEvent[*corev1.Pod]) bool { return false },
				UpdateFunc: func(e event.TypedUpdateEvent[*corev1.Pod]) bool {
					return e.ObjectOld.Status.Phase != e.ObjectNew.Status.Phase
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on Pod: %w", err)
	}
	return nil
}
