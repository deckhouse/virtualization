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
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type PersistentVolumeClaimWatcher struct{}

func NewPersistentVolumeClaimWatcher() *PersistentVolumeClaimWatcher {
	return &PersistentVolumeClaimWatcher{}
}

func (w *PersistentVolumeClaimWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &corev1.PersistentVolumeClaim{},
			handler.TypedEnqueueRequestForOwner[*corev1.PersistentVolumeClaim](
				mgr.GetScheme(),
				mgr.GetRESTMapper(),
				&v1alpha2.VirtualImage{},
			), predicate.TypedFuncs[*corev1.PersistentVolumeClaim]{
				UpdateFunc: func(e event.TypedUpdateEvent[*corev1.PersistentVolumeClaim]) bool {
					if e.ObjectOld.Status.Capacity[corev1.ResourceStorage] != e.ObjectNew.Status.Capacity[corev1.ResourceStorage] {
						return true
					}

					if service.GetPersistentVolumeClaimCondition(corev1.PersistentVolumeClaimResizing, e.ObjectOld.Status.Conditions) != nil ||
						service.GetPersistentVolumeClaimCondition(corev1.PersistentVolumeClaimResizing, e.ObjectNew.Status.Conditions) != nil {
						return true
					}

					return e.ObjectOld.Status.Phase != e.ObjectNew.Status.Phase && e.ObjectNew.Status.Phase == corev1.ClaimBound
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on PVC: %w", err)
	}
	return nil
}
