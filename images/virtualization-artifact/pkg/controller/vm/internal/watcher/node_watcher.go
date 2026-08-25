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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// NodeWatcher wakes up virtual machines when readiness of their node changes. Nothing else
// reports that a running virtual machine became unobservable: the node is gone, so neither the
// virtual machine nor its pod generates events any more.
func NewNodeWatcher(client client.Client) *NodeWatcher {
	return &NodeWatcher{client: client}
}

type NodeWatcher struct {
	client client.Client
}

func (w *NodeWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(
			mgr.GetCache(),
			&corev1.Node{},
			handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context, node *corev1.Node) []reconcile.Request {
				var vms v1alpha2.VirtualMachineList
				if err := w.client.List(ctx, &vms, client.MatchingFields{indexer.IndexFieldVMByNode: node.GetName()}); err != nil {
					logger.FromContext(ctx).Error(fmt.Sprintf("failed to list virtual machines on node %s: %s", node.GetName(), err))
					return nil
				}

				requests := make([]reconcile.Request, 0, len(vms.Items))
				for _, vm := range vms.Items {
					requests = append(requests, reconcile.Request{
						NamespacedName: types.NamespacedName{Name: vm.GetName(), Namespace: vm.GetNamespace()},
					})
				}
				return requests
			}),
			predicate.TypedFuncs[*corev1.Node]{
				CreateFunc: func(event.TypedCreateEvent[*corev1.Node]) bool { return false },
				DeleteFunc: func(event.TypedDeleteEvent[*corev1.Node]) bool { return true },
				UpdateFunc: func(e event.TypedUpdateEvent[*corev1.Node]) bool {
					return nodeReady(e.ObjectOld) != nodeReady(e.ObjectNew)
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on Node: %w", err)
	}

	return nil
}

func nodeReady(node *corev1.Node) corev1.ConditionStatus {
	if node == nil {
		return corev1.ConditionUnknown
	}
	for _, c := range node.Status.Conditions {
		if c.Type == corev1.NodeReady {
			return c.Status
		}
	}
	return corev1.ConditionUnknown
}
