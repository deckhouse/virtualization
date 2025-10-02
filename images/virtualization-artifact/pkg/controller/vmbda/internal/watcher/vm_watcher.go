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
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
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
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &v1alpha2.VirtualMachine{},
			handler.TypedEnqueueRequestsFromMapFunc(w.enqueueRequests),
			predicate.TypedFuncs[*v1alpha2.VirtualMachine]{
				CreateFunc: func(e event.TypedCreateEvent[*v1alpha2.VirtualMachine]) bool { return false },
				UpdateFunc: func(e event.TypedUpdateEvent[*v1alpha2.VirtualMachine]) bool {
					oldRunningCondition, _ := conditions.GetCondition(vmcondition.TypeRunning, e.ObjectOld.Status.Conditions)
					newRunningCondition, _ := conditions.GetCondition(vmcondition.TypeRunning, e.ObjectNew.Status.Conditions)

					if oldRunningCondition.Status != newRunningCondition.Status || e.ObjectOld.Status.Phase != e.ObjectNew.Status.Phase {
						return true
					}

					return w.hasBlockDeviceAttachmentChanges(e.ObjectOld, e.ObjectNew)
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VMs: %w", err)
	}
	return nil
}

func (w VirtualMachineWatcher) enqueueRequests(ctx context.Context, vm *v1alpha2.VirtualMachine) (requests []reconcile.Request) {
	var vmbdas v1alpha2.VirtualMachineBlockDeviceAttachmentList
	err := w.client.List(ctx, &vmbdas, &client.ListOptions{
		Namespace: vm.GetNamespace(),
	})
	if err != nil {
		slog.Default().Error(fmt.Sprintf("failed to list vmbdas: %s", err))
		return
	}

	for _, vmbda := range vmbdas.Items {
		if vmbda.Spec.VirtualMachineName != vm.GetName() {
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

func (w VirtualMachineWatcher) hasBlockDeviceAttachmentChanges(oldVM, newVM *v1alpha2.VirtualMachine) bool {
	var oldVMBDA []v1alpha2.BlockDeviceStatusRef
	for _, bdRef := range oldVM.Status.BlockDeviceRefs {
		if bdRef.VirtualMachineBlockDeviceAttachmentName != "" {
			oldVMBDA = append(oldVMBDA, bdRef)
		}
	}

	var newVMBDA []v1alpha2.BlockDeviceStatusRef
	for _, bdRef := range newVM.Status.BlockDeviceRefs {
		if bdRef.VirtualMachineBlockDeviceAttachmentName != "" {
			newVMBDA = append(newVMBDA, bdRef)
		}
	}

	return !reflect.DeepEqual(oldVMBDA, newVMBDA)
}
