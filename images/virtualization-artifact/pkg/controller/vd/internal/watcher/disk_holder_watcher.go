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

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// The watchers below wake the owner election of a disk: an attachment claims the disk, and the volume
// on the instance or a pending attach request of the internal VM keeps it until KubeVirt detaches it.

type VirtualMachineBlockDeviceAttachmentWatcher struct{}

func NewVirtualMachineBlockDeviceAttachmentWatcher() *VirtualMachineBlockDeviceAttachmentWatcher {
	return &VirtualMachineBlockDeviceAttachmentWatcher{}
}

func (w *VirtualMachineBlockDeviceAttachmentWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &v1alpha2.VirtualMachineBlockDeviceAttachment{},
			handler.TypedEnqueueRequestsFromMapFunc(enqueueAttachedDisk),
			predicate.TypedFuncs[*v1alpha2.VirtualMachineBlockDeviceAttachment]{
				UpdateFunc: func(e event.TypedUpdateEvent[*v1alpha2.VirtualMachineBlockDeviceAttachment]) bool {
					return attachmentChangeMattersToDisk(e.ObjectOld, e.ObjectNew)
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VMBDAs: %w", err)
	}
	return nil
}

// attachmentChangeMattersToDisk also wakes the disk on a condition change: the attachment of a stopped VM
// stays Pending when the VM starts, and only its VirtualMachineReady condition shows the start.
func attachmentChangeMattersToDisk(oldVMBDA, newVMBDA *v1alpha2.VirtualMachineBlockDeviceAttachment) bool {
	return oldVMBDA.Status.Phase != newVMBDA.Status.Phase ||
		oldVMBDA.DeletionTimestamp.IsZero() != newVMBDA.DeletionTimestamp.IsZero() ||
		!equality.Semantic.DeepEqual(oldVMBDA.Status.Conditions, newVMBDA.Status.Conditions)
}

func enqueueAttachedDisk(_ context.Context, vmbda *v1alpha2.VirtualMachineBlockDeviceAttachment) []reconcile.Request {
	if vmbda.Spec.BlockDeviceRef.Kind != v1alpha2.VMBDAObjectRefKindVirtualDisk {
		return nil
	}

	return []reconcile.Request{{NamespacedName: types.NamespacedName{
		Namespace: vmbda.Namespace,
		Name:      vmbda.Spec.BlockDeviceRef.Name,
	}}}
}

type KVVMIWatcher struct {
	client client.Client
}

func NewKVVMIWatcher(client client.Client) *KVVMIWatcher {
	return &KVVMIWatcher{client: client}
}

func (w *KVVMIWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv1.VirtualMachineInstance{},
			volumeNamesHandler(w.client, instanceVolumeNames),
		),
	); err != nil {
		return fmt.Errorf("error setting watch on KVVMIs: %w", err)
	}
	return nil
}

type KVVMWatcher struct {
	client client.Client
}

func NewKVVMWatcher(client client.Client) *KVVMWatcher {
	return &KVVMWatcher{client: client}
}

func (w *KVVMWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv1.VirtualMachine{},
			volumeNamesHandler(w.client, requestedVolumeNames),
		),
	); err != nil {
		return fmt.Errorf("error setting watch on KVVMs: %w", err)
	}
	return nil
}

func instanceVolumeNames(kvvmi *virtv1.VirtualMachineInstance) map[string]struct{} {
	names := make(map[string]struct{}, len(kvvmi.Status.VolumeStatus)+len(kvvmi.Spec.Volumes))
	for _, vs := range kvvmi.Status.VolumeStatus {
		names[vs.Name] = struct{}{}
	}
	for _, v := range kvvmi.Spec.Volumes {
		names[v.Name] = struct{}{}
	}
	return names
}

func requestedVolumeNames(kvvm *virtv1.VirtualMachine) map[string]struct{} {
	names := make(map[string]struct{}, len(kvvm.Status.VolumeRequests))
	for _, vr := range kvvm.Status.VolumeRequests {
		if vr.AddVolumeOptions != nil {
			names[vr.AddVolumeOptions.Name] = struct{}{}
		}
	}
	return names
}

type handlerQueue = workqueue.TypedRateLimitingInterface[reconcile.Request]

// volumeNamesHandler enqueues the disks whose volumes appeared on or left the object: a phase change of a
// volume that stays does not change who holds the disk.
func volumeNamesHandler[T client.Object](c client.Client, volumeNames func(T) map[string]struct{}) handler.TypedEventHandler[T, reconcile.Request] {
	return handler.TypedFuncs[T, reconcile.Request]{
		CreateFunc: func(ctx context.Context, e event.TypedCreateEvent[T], q handlerQueue) {
			enqueueDisksByVolume(ctx, c, e.Object.GetNamespace(), volumeNames(e.Object), q)
		},
		UpdateFunc: func(ctx context.Context, e event.TypedUpdateEvent[T], q handlerQueue) {
			enqueueDisksByVolume(ctx, c, e.ObjectNew.GetNamespace(), changedVolumeNames(volumeNames(e.ObjectOld), volumeNames(e.ObjectNew)), q)
		},
		DeleteFunc: func(ctx context.Context, e event.TypedDeleteEvent[T], q handlerQueue) {
			enqueueDisksByVolume(ctx, c, e.Object.GetNamespace(), volumeNames(e.Object), q)
		},
	}
}

func changedVolumeNames(old, cur map[string]struct{}) map[string]struct{} {
	changed := make(map[string]struct{})
	for name := range cur {
		if _, ok := old[name]; !ok {
			changed[name] = struct{}{}
		}
	}
	for name := range old {
		if _, ok := cur[name]; !ok {
			changed[name] = struct{}{}
		}
	}
	return changed
}

// enqueueDisksByVolume matches disks by the derived volume name: shortened names cannot be mapped back.
func enqueueDisksByVolume(ctx context.Context, c client.Client, ns string, volumeNames map[string]struct{}, q handlerQueue) {
	for _, req := range disksByVolume(ctx, c, ns, volumeNames) {
		q.Add(req)
	}
}

func disksByVolume(ctx context.Context, c client.Client, ns string, volumeNames map[string]struct{}) []reconcile.Request {
	var requests []reconcile.Request
	for name := range volumeNames {
		var vds v1alpha2.VirtualDiskList
		if err := c.List(ctx, &vds, client.InNamespace(ns), client.MatchingFields{indexer.IndexFieldVDByVolumeName: name}); err != nil {
			slog.Default().Error(fmt.Sprintf("failed to list virtual disks by volume %s: %s", name, err))
			continue
		}
		for _, vd := range vds.Items {
			requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{Namespace: vd.Namespace, Name: vd.Name}})
		}
	}
	return requests
}
