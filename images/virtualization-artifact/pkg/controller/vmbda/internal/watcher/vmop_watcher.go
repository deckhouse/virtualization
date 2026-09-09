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
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

type VirtualMachineOperationWatcher struct {
	client client.Client
}

func NewVirtualMachineOperationWatcher(client client.Client) *VirtualMachineOperationWatcher {
	return &VirtualMachineOperationWatcher{client: client}
}

func (w VirtualMachineOperationWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &v1alpha2.VirtualMachineOperation{},
			handler.TypedEnqueueRequestsFromMapFunc(w.enqueueRequests),
			predicate.TypedFuncs[*v1alpha2.VirtualMachineOperation]{
				CreateFunc: func(e event.TypedCreateEvent[*v1alpha2.VirtualMachineOperation]) bool {
					return commonvmop.IsMigration(e.Object)
				},
				UpdateFunc: func(e event.TypedUpdateEvent[*v1alpha2.VirtualMachineOperation]) bool {
					if !commonvmop.IsMigration(e.ObjectNew) {
						return false
					}

					if e.ObjectOld.Status.Phase != e.ObjectNew.Status.Phase {
						return true
					}

					oldCompleted, _ := conditions.GetCondition(vmopcondition.TypeCompleted, e.ObjectOld.Status.Conditions)
					newCompleted, _ := conditions.GetCondition(vmopcondition.TypeCompleted, e.ObjectNew.Status.Conditions)

					return oldCompleted.Reason != newCompleted.Reason
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualMachineOperation: %w", err)
	}
	return nil
}

func (w VirtualMachineOperationWatcher) enqueueRequests(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (requests []reconcile.Request) {
	var vmbdas v1alpha2.VirtualMachineBlockDeviceAttachmentList
	err := w.client.List(ctx, &vmbdas,
		client.InNamespace(vmop.GetNamespace()),
		client.MatchingFields{indexer.IndexFieldVMBDAByVM: vmop.Spec.VirtualMachine},
	)
	if err != nil {
		slog.Default().Error(fmt.Sprintf("failed to list vmbdas: %s", err))
		return requests
	}

	for _, vmbda := range vmbdas.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      vmbda.Name,
				Namespace: vmbda.Namespace,
			},
		})
	}

	return requests
}
