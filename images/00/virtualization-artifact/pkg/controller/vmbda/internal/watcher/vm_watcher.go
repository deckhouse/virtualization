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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

type VirtualMachineWatcher struct {
	client client.Client
}

func NewVirtualMachineWatcher(client client.Client) *VirtualMachineWatcher {
	return &VirtualMachineWatcher{
		client: client,
	}
}

func (w VirtualMachineWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	return ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.VirtualMachine{}),
		handler.EnqueueRequestsFromMapFunc(w.enqueueRequests),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return false },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: w.filterUpdateEvents,
		},
	)
}

func (w VirtualMachineWatcher) enqueueRequests(ctx context.Context, obj client.Object) (requests []reconcile.Request) {
	var vmbdas virtv2.VirtualMachineBlockDeviceAttachmentList
	err := w.client.List(ctx, &vmbdas, &client.ListOptions{
		Namespace: obj.GetNamespace(),
	})
	if err != nil {
		slog.Default().Error(fmt.Sprintf("failed to list vmbdas: %s", err))
		return
	}

	for _, vmbda := range vmbdas.Items {
		if vmbda.Spec.VirtualMachineName != obj.GetName() {
			continue
		}

		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      vmbda.Name,
				Namespace: vmbda.Namespace,
			},
		})
	}

	return
}

func (w VirtualMachineWatcher) filterUpdateEvents(e event.UpdateEvent) bool {
	oldVM, ok := e.ObjectOld.(*virtv2.VirtualMachine)
	if !ok {
		slog.Default().Error(fmt.Sprintf("expected an old VirtualMachine but got a %T", e.ObjectOld))
		return false
	}

	newVM, ok := e.ObjectNew.(*virtv2.VirtualMachine)
	if !ok {
		slog.Default().Error(fmt.Sprintf("expected a new VirtualMachine but got a %T", e.ObjectNew))
		return false
	}

	oldRunningCondition, _ := conditions.GetCondition(vmcondition.TypeRunning, oldVM.Status.Conditions)
	newRunningCondition, _ := conditions.GetCondition(vmcondition.TypeRunning, newVM.Status.Conditions)

	if newRunningCondition.Status != oldRunningCondition.Status {
		return true
	}

	return w.hasBlockDeviceAttachmentChanges(oldVM, newVM)
}

func (w VirtualMachineWatcher) hasBlockDeviceAttachmentChanges(oldVM, newVM *virtv2.VirtualMachine) bool {
	var oldVMBDA []virtv2.BlockDeviceStatusRef
	for _, bdRef := range oldVM.Status.BlockDeviceRefs {
		if bdRef.VirtualMachineBlockDeviceAttachmentName != "" {
			oldVMBDA = append(oldVMBDA, bdRef)
		}
	}

	var newVMBDA []virtv2.BlockDeviceStatusRef
	for _, bdRef := range newVM.Status.BlockDeviceRefs {
		if bdRef.VirtualMachineBlockDeviceAttachmentName != "" {
			newVMBDA = append(newVMBDA, bdRef)
		}
	}

	return !reflect.DeepEqual(oldVMBDA, newVMBDA)
}
