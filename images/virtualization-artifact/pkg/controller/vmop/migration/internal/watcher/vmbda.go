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

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	commonvmop "github.com/deckhouse/virtualization-controller/pkg/common/vmop"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func NewVMBDAWatcher() *VMBDAWatcher {
	return &VMBDAWatcher{}
}

type VMBDAWatcher struct{}

func (w VMBDAWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	mgrClient := mgr.GetClient()
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &v1alpha2.VirtualMachineBlockDeviceAttachment{},
			handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context, vmbda *v1alpha2.VirtualMachineBlockDeviceAttachment) []reconcile.Request {
				vmops := &v1alpha2.VirtualMachineOperationList{}
				if err := mgrClient.List(ctx, vmops, client.InNamespace(vmbda.GetNamespace())); err != nil {
					return nil
				}

				var requests []reconcile.Request
				for _, vmop := range vmops.Items {
					if !Match(&vmop) || commonvmop.IsFinished(&vmop) {
						continue
					}

					if vmop.Spec.VirtualMachine == vmbda.Spec.VirtualMachineName {
						requests = append(requests, reconcile.Request{
							NamespacedName: types.NamespacedName{
								Namespace: vmop.GetNamespace(),
								Name:      vmop.GetName(),
							},
						})
					}
				}

				return requests
			}),
			predicate.TypedFuncs[*v1alpha2.VirtualMachineBlockDeviceAttachment]{
				CreateFunc: func(e event.TypedCreateEvent[*v1alpha2.VirtualMachineBlockDeviceAttachment]) bool { return false },
				UpdateFunc: func(e event.TypedUpdateEvent[*v1alpha2.VirtualMachineBlockDeviceAttachment]) bool {
					return e.ObjectOld.Status.Phase != e.ObjectNew.Status.Phase
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualMachineBlockDeviceAttachment: %w", err)
	}
	return nil
}
