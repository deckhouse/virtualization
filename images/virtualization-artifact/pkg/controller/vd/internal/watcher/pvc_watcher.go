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
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/common/datavolume"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type PersistentVolumeClaimWatcher struct {
	logger *log.Logger
	client client.Client
}

func NewPersistentVolumeClaimWatcher(client client.Client) *PersistentVolumeClaimWatcher {
	return &PersistentVolumeClaimWatcher{
		logger: log.Default().With("watcher", strings.ToLower("PersistentVolumeClaim")),
		client: client,
	}
}

func (w PersistentVolumeClaimWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	return ctr.Watch(
		source.Kind(mgr.GetCache(), &corev1.PersistentVolumeClaim{}),
		handler.EnqueueRequestsFromMapFunc(w.enqueueRequestsFromOwnerRefsRecursively),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: w.filterUpdateEvents,
		},
	)
}

func (w PersistentVolumeClaimWatcher) enqueueRequestsFromOwnerRefsRecursively(ctx context.Context, obj client.Object) (requests []reconcile.Request) {
	for _, ownerRef := range obj.GetOwnerReferences() {
		switch ownerRef.Kind {
		case virtv2.VirtualDiskKind:
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      ownerRef.Name,
					Namespace: obj.GetNamespace(),
				},
			})
		case datavolume.DataVolumeKind:
			dv, err := object.FetchObject(ctx, types.NamespacedName{
				Name:      ownerRef.Name,
				Namespace: obj.GetNamespace(),
			}, w.client, &cdiv1.DataVolume{})
			if err != nil {
				w.logger.Error(fmt.Sprintf("failed to fetch dv: %s", err))
				continue
			}

			if dv == nil {
				continue
			}

			requests = append(requests, w.enqueueRequestsFromOwnerRefsRecursively(ctx, dv)...)
		}
	}

	return
}

func (w PersistentVolumeClaimWatcher) filterUpdateEvents(e event.UpdateEvent) bool {
	oldPVC, ok := e.ObjectOld.(*corev1.PersistentVolumeClaim)
	if !ok {
		return false
	}
	newPVC, ok := e.ObjectNew.(*corev1.PersistentVolumeClaim)
	if !ok {
		return false
	}

	if oldPVC.Status.Capacity[corev1.ResourceStorage] != newPVC.Status.Capacity[corev1.ResourceStorage] {
		return true
	}

	if service.GetPersistentVolumeClaimCondition(corev1.PersistentVolumeClaimResizing, oldPVC.Status.Conditions) != nil ||
		service.GetPersistentVolumeClaimCondition(corev1.PersistentVolumeClaimResizing, newPVC.Status.Conditions) != nil {
		return true
	}

	return oldPVC.Status.Phase != newPVC.Status.Phase
}
