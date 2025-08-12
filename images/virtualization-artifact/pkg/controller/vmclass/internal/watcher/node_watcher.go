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
	"maps"
	"slices"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/component-helpers/scheduling/corev1/nodeaffinity"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type NodesWatcher struct{}

func NewNodesWatcher() *NodesWatcher {
	return &NodesWatcher{}
}

func (w *NodesWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &corev1.Node{},
			handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context, node *corev1.Node) []reconcile.Request {
				var result []reconcile.Request

				classList := &virtv2.VirtualMachineClassList{}
				err := mgr.GetClient().List(ctx, classList)
				if err != nil {
					log.Error("failed to list VMClasses", "error", err)
					return nil
				}

				for _, class := range classList.Items {
					if slices.Contains(class.Status.AvailableNodes, node.GetName()) {
						result = append(result, reconcile.Request{
							NamespacedName: object.NamespacedName(&class),
						})
						continue
					}

					if len(class.Spec.NodeSelector.MatchLabels) != 0 {
						if !annotations.MatchLabels(node.GetLabels(), class.Spec.NodeSelector.MatchLabels) {
							continue
						}
					}

					if len(class.Spec.NodeSelector.MatchExpressions) != 0 {
						ns, err := nodeaffinity.NewNodeSelector(&corev1.NodeSelector{
							NodeSelectorTerms: []corev1.NodeSelectorTerm{{MatchExpressions: class.Spec.NodeSelector.MatchExpressions}},
						})
						if err != nil {
							log.Error("failed to parse NodeSelector", log.Err(err))
							continue
						}

						if !ns.Match(node) {
							continue
						}
					}

					for _, feature := range class.Spec.CPU.Features {
						v, ok := node.Annotations[annotations.AnnNodeCPUFeature+feature]
						if !ok || v != "true" {
							continue
						}
					}

					result = append(result, reconcile.Request{
						NamespacedName: object.NamespacedName(&class),
					})
				}
				return result
			}),
			predicate.Or(
				predicate.TypedLabelChangedPredicate[*corev1.Node]{},
				predicate.TypedFuncs[*corev1.Node]{
					UpdateFunc: func(e event.TypedUpdateEvent[*corev1.Node]) bool {
						if !maps.Equal(e.ObjectOld.Annotations, e.ObjectNew.Annotations) {
							return true
						}
						if !e.ObjectOld.Status.Allocatable[corev1.ResourceCPU].Equal(e.ObjectNew.Status.Allocatable[corev1.ResourceCPU]) {
							return true
						}
						if !e.ObjectOld.Status.Allocatable[corev1.ResourceMemory].Equal(e.ObjectNew.Status.Allocatable[corev1.ResourceMemory]) {
							return true
						}
						return false
					},
				},
			),
		),
	); err != nil {
		return fmt.Errorf("error setting watch on Nodes: %w", err)
	}
	return nil
}
