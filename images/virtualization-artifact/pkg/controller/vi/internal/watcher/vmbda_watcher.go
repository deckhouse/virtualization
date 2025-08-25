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

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/deckhouse/pkg/log"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VirtualMachineBlockDeviceAttachmentWatcher struct {
	logger *log.Logger
	client client.Client
}

func NewVirtualMachineBlockDeviceAttachmentWatcher(client client.Client) *VirtualMachineBlockDeviceAttachmentWatcher {
	return &VirtualMachineBlockDeviceAttachmentWatcher{
		logger: log.Default().With("watcher", "vmbda"),
		client: client,
	}
}

func (w VirtualMachineBlockDeviceAttachmentWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	return ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.VirtualMachineBlockDeviceAttachment{},
			handler.TypedEnqueueRequestsFromMapFunc(w.enqueueRequests),
			predicate.TypedFuncs[*virtv2.VirtualMachineBlockDeviceAttachment]{
				CreateFunc: func(e event.TypedCreateEvent[*virtv2.VirtualMachineBlockDeviceAttachment]) bool {
					return w.isVirtualImageRef(e.Object)
				},
				DeleteFunc: func(e event.TypedDeleteEvent[*virtv2.VirtualMachineBlockDeviceAttachment]) bool {
					return w.isVirtualImageRef(e.Object)
				},
				UpdateFunc: func(e event.TypedUpdateEvent[*virtv2.VirtualMachineBlockDeviceAttachment]) bool {
					return w.isVirtualImageRef(e.ObjectOld) || w.isVirtualImageRef(e.ObjectNew)
				},
			},
		),
	)
}

func (w VirtualMachineBlockDeviceAttachmentWatcher) enqueueRequests(ctx context.Context, vmbda *virtv2.VirtualMachineBlockDeviceAttachment) (requests []reconcile.Request) {
	requests = append(requests, reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      vmbda.Spec.BlockDeviceRef.Name,
			Namespace: vmbda.Namespace,
		},
	})

	return
}

func (w VirtualMachineBlockDeviceAttachmentWatcher) isVirtualImageRef(vmbda *virtv2.VirtualMachineBlockDeviceAttachment) bool {
	return vmbda.Spec.BlockDeviceRef.Kind == virtv2.VirtualImageKind
}
