/*
Copyright 2024 Flant JSC

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
	"reflect"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type KVVMIWatcher struct {
	client client.Client
}

var _ handler.EventHandler = &KVVMIEventHandler{}

func NewKVVMIWatcher(client client.Client) *KVVMIWatcher {
	return &KVVMIWatcher{
		client: client,
	}
}

func (w KVVMIWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	return ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv1.VirtualMachineInstance{}),
		NewKVVMIEventHandler(w.client),
	)
}

type KVVMIEventHandler struct {
	client client.Client
}

func NewKVVMIEventHandler(client client.Client) *KVVMIEventHandler {
	return &KVVMIEventHandler{
		client: client,
	}
}

func (eh KVVMIEventHandler) Create(ctx context.Context, e event.CreateEvent, q workqueue.RateLimitingInterface) {
	eh.enqueueRequests(ctx, e.Object.GetNamespace(), eh.getHotPluggedVolumeStatuses(e.Object), q)
}

func (eh KVVMIEventHandler) Delete(ctx context.Context, e event.DeleteEvent, q workqueue.RateLimitingInterface) {
	eh.enqueueRequests(ctx, e.Object.GetNamespace(), eh.getHotPluggedVolumeStatuses(e.Object), q)
}

func (eh KVVMIEventHandler) Update(ctx context.Context, e event.UpdateEvent, q workqueue.RateLimitingInterface) {
	oldVolumeStatuses := eh.getHotPluggedVolumeStatuses(e.ObjectOld)
	newVolumeStatuses := eh.getHotPluggedVolumeStatuses(e.ObjectNew)

	eh.enqueueRequests(ctx, e.ObjectNew.GetNamespace(), getVolumeStatusesToReconcile(oldVolumeStatuses, newVolumeStatuses), q)
}

func (eh KVVMIEventHandler) Generic(_ context.Context, _ event.GenericEvent, _ workqueue.RateLimitingInterface) {
	// Not implemented.
}

func (eh KVVMIEventHandler) enqueueRequests(ctx context.Context, ns string, vsToReconcile map[string]virtv1.VolumeStatus, q workqueue.RateLimitingInterface) {
	if len(vsToReconcile) == 0 {
		return
	}

	var vmbdas virtv2.VirtualMachineBlockDeviceAttachmentList
	err := eh.client.List(ctx, &vmbdas, &client.ListOptions{
		Namespace: ns,
	})
	if err != nil {
		slog.Default().Error(fmt.Sprintf("failed to list vmbdas: %s", err))
		return
	}

	for _, vmbda := range vmbdas.Items {
		_, ok := vsToReconcile[vmbda.Spec.BlockDeviceRef.Name]
		if ok {
			q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
				Namespace: vmbda.Namespace,
				Name:      vmbda.Name,
			}})
		}
	}
}

func (eh KVVMIEventHandler) getHotPluggedVolumeStatuses(obj client.Object) map[string]virtv1.VolumeStatus {
	kvvmi, ok := obj.(*virtv1.VirtualMachineInstance)
	if !ok || kvvmi == nil {
		return nil
	}

	volumeStatuses := make(map[string]virtv1.VolumeStatus)

	for _, vs := range kvvmi.Status.VolumeStatus {
		if vs.HotplugVolume != nil {
			name, kind := kvbuilder.GetOriginalDiskName(vs.Name)
			if kind == "" {
				slog.Default().Warn("VolumeStatus is not a Disk", "vsName", vs.Name, "name", kvvmi.Name, "ns", kvvmi.Namespace)
				continue
			}

			volumeStatuses[name] = vs
		}
	}

	return volumeStatuses
}

func getVolumeStatusesToReconcile(oldVolumeStatuses, newVolumeStatuses map[string]virtv1.VolumeStatus) map[string]virtv1.VolumeStatus {
	result := make(map[string]virtv1.VolumeStatus)

	for vsName, newVS := range newVolumeStatuses {
		if oldVS, ok := oldVolumeStatuses[vsName]; !ok || !reflect.DeepEqual(oldVS, newVS) {
			result[vsName] = newVS
		}
	}

	for vsName, oldVS := range oldVolumeStatuses {
		if _, ok := newVolumeStatuses[vsName]; !ok {
			result[vsName] = oldVS
		}
	}

	return result
}
