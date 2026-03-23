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
	"log/slog"

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

const reasonFailedAttachVolume = "FailedAttachVolume"

type VolumeEventWatcher struct {
	client client.Client
}

func NewVolumeEventWatcher(client client.Client) *VolumeEventWatcher {
	return &VolumeEventWatcher{client: client}
}

func (w *VolumeEventWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(
			mgr.GetCache(),
			&corev1.Event{},
			handler.TypedEnqueueRequestsFromMapFunc(w.enqueueVMBDAs),
			predicate.TypedFuncs[*corev1.Event]{
				CreateFunc: func(e event.TypedCreateEvent[*corev1.Event]) bool {
					return isFailedAttachVolumeEvent(e.Object)
				},
				UpdateFunc: func(e event.TypedUpdateEvent[*corev1.Event]) bool {
					return isFailedAttachVolumeEvent(e.ObjectNew)
				},
				DeleteFunc: func(_ event.TypedDeleteEvent[*corev1.Event]) bool {
					return false
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on Event: %w", err)
	}
	return nil
}

func isFailedAttachVolumeEvent(e *corev1.Event) bool {
	return e.InvolvedObject.Kind == "Pod" &&
		e.Type == corev1.EventTypeWarning &&
		e.Reason == reasonFailedAttachVolume
}

func (w *VolumeEventWatcher) enqueueVMBDAs(ctx context.Context, e *corev1.Event) []reconcile.Request {
	if e.InvolvedObject.Kind != "Pod" {
		return nil
	}

	ns := e.InvolvedObject.Namespace
	podName := e.InvolvedObject.Name

	var kvvmiList virtv1.VirtualMachineInstanceList
	if err := w.client.List(ctx, &kvvmiList, &client.ListOptions{Namespace: ns}); err != nil {
		slog.Default().Error(fmt.Sprintf("failed to list kvvmis: %s", err))
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
				slog.Default().Error(fmt.Sprintf("failed to list vmbdas: %s", err))
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
