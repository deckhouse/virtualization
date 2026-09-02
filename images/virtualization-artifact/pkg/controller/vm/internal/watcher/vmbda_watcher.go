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
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// VMBDAWatcher wakes the virtual machine up while a block device is being hot-plugged into it or
// unplugged from it, so that the OperationInProgress condition of the machine follows the
// attachment.
type VMBDAWatcher struct{}

func NewVMBDAWatcher() *VMBDAWatcher {
	return &VMBDAWatcher{}
}

func (w VMBDAWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(
			mgr.GetCache(),
			&v1alpha2.VirtualMachineBlockDeviceAttachment{},
			handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context, vmbda *v1alpha2.VirtualMachineBlockDeviceAttachment) []reconcile.Request {
				return []reconcile.Request{
					{
						NamespacedName: types.NamespacedName{
							Name:      vmbda.Spec.VirtualMachineName,
							Namespace: vmbda.GetNamespace(),
						},
					},
				}
			}),
			predicate.TypedFuncs[*v1alpha2.VirtualMachineBlockDeviceAttachment]{
				UpdateFunc: func(e event.TypedUpdateEvent[*v1alpha2.VirtualMachineBlockDeviceAttachment]) bool {
					return e.ObjectOld.Status.Phase != e.ObjectNew.Status.Phase ||
						e.ObjectOld.GetDeletionTimestamp().IsZero() != e.ObjectNew.GetDeletionTimestamp().IsZero()
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualMachineBlockDeviceAttachment: %w", err)
	}
	return nil
}
