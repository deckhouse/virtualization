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

	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
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

type KVVMIWatcher struct{}

func NewKVVMIWatcher() *KVVMIWatcher {
	return &KVVMIWatcher{}
}

func (w *KVVMIWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv1.VirtualMachineInstance{},
			handler.TypedEnqueueRequestsFromMapFunc(w.enqueueDisksWithNotZeroSize),
			predicate.TypedFuncs[*virtv1.VirtualMachineInstance]{
				CreateFunc: func(e event.TypedCreateEvent[*virtv1.VirtualMachineInstance]) bool {
					return false
				},
				DeleteFunc: func(e event.TypedDeleteEvent[*virtv1.VirtualMachineInstance]) bool {
					return false
				},
				UpdateFunc: func(e event.TypedUpdateEvent[*virtv1.VirtualMachineInstance]) bool {
					return w.someDiskSizeChanged(e.ObjectOld, e.ObjectNew)
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VMs: %w", err)
	}
	return nil
}

func (w *KVVMIWatcher) someDiskSizeChanged(oldVMI, newVMI *virtv1.VirtualMachineInstance) bool {
	oldVolumeSizes := make(map[string]int64)
	newVolumeSizes := make(map[string]int64)

	for _, volume := range oldVMI.Status.VolumeStatus {
		name, kind := kvbuilder.GetOriginalDiskName(volume.Name)
		if kind == v1alpha2.DiskDevice {
			oldVolumeSizes[name] = volume.Size
		}
	}
	for _, volume := range newVMI.Status.VolumeStatus {
		name, kind := kvbuilder.GetOriginalDiskName(volume.Name)
		if kind == v1alpha2.DiskDevice {
			newVolumeSizes[name] = volume.Size
		}
	}

	for newName, newSize := range newVolumeSizes {
		oldSize := oldVolumeSizes[newName]
		if newSize > 0 && newSize != oldSize {
			return true
		}
	}

	return false
}

func (w *KVVMIWatcher) enqueueDisksWithNotZeroSize(_ context.Context, vmi *virtv1.VirtualMachineInstance) []reconcile.Request {
	var requests []reconcile.Request

	for _, volumeStatus := range vmi.Status.VolumeStatus {
		name, kind := kvbuilder.GetOriginalDiskName(volumeStatus.Name)
		if kind == v1alpha2.DiskDevice && volumeStatus.Size > 0 {
			requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{
				Name:      name,
				Namespace: vmi.Namespace,
			}})
		}
	}

	return requests
}
