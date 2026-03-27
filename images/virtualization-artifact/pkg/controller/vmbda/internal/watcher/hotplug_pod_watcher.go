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
	"reflect"

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

	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func NewHotPlugPodWatcher(client client.Client) *HotPlugPodWatcher {
	return &HotPlugPodWatcher{
		client: client,
	}
}

type HotPlugPodWatcher struct {
	client client.Client
}

func (w *HotPlugPodWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(
			mgr.GetCache(),
			&corev1.Pod{},
			handler.TypedEnqueueRequestsFromMapFunc(w.enqueueVMBDAs),
			predicate.TypedFuncs[*corev1.Pod]{
				CreateFunc: func(e event.TypedCreateEvent[*corev1.Pod]) bool { return true },
				DeleteFunc: func(e event.TypedDeleteEvent[*corev1.Pod]) bool { return true },
				UpdateFunc: func(e event.TypedUpdateEvent[*corev1.Pod]) bool {
					return e.ObjectOld.Status.Phase != e.ObjectNew.Status.Phase ||
						!reflect.DeepEqual(e.ObjectOld.Status.Conditions, e.ObjectNew.Status.Conditions) ||
						!reflect.DeepEqual(e.ObjectOld.Status.ContainerStatuses, e.ObjectNew.Status.ContainerStatuses)
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on hot-plug Pod: %w", err)
	}
	return nil
}

func (w *HotPlugPodWatcher) enqueueVMBDAs(ctx context.Context, pod *corev1.Pod) []reconcile.Request {
	if pod == nil {
		return nil
	}

	ns := pod.Namespace
	podName := pod.Name

	var kvvmiList virtv1.VirtualMachineInstanceList
	if err := w.client.List(ctx, &kvvmiList, &client.ListOptions{Namespace: ns}); err != nil {
		return nil
	}

	for _, kvvmi := range kvvmiList.Items {
		for _, vs := range kvvmi.Status.VolumeStatus {
			if vs.HotplugVolume == nil || vs.HotplugVolume.AttachPodName != podName {
				continue
			}

			name, kind := kvbuilder.GetOriginalDiskName(vs.Name)
			if kind == "" {
				continue
			}

			var vmbdas v1alpha2.VirtualMachineBlockDeviceAttachmentList
			if err := w.client.List(ctx, &vmbdas, &client.ListOptions{Namespace: ns}); err != nil {
				return nil
			}

			var requests []reconcile.Request
			for _, vmbda := range vmbdas.Items {
				if vmbda.Spec.BlockDeviceRef.Name == name {
					requests = append(requests, reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: vmbda.Namespace,
							Name:      vmbda.Name,
						},
					})
				}
			}
			return requests
		}
	}

	return nil
}
