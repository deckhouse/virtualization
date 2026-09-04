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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
)

func NewNodeWatcher() *NodeWatcher {
	return &NodeWatcher{}
}

type NodeWatcher struct{}

func (w *NodeWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(
			mgr.GetCache(),
			&corev1.Node{},
			&handler.TypedEnqueueRequestForObject[*corev1.Node]{},
			predicate.TypedFuncs[*corev1.Node]{
				CreateFunc: func(event.TypedCreateEvent[*corev1.Node]) bool { return true },
				UpdateFunc: func(e event.TypedUpdateEvent[*corev1.Node]) bool {
					return dialogueChanged(e.ObjectOld, e.ObjectNew)
				},
				DeleteFunc: func(event.TypedDeleteEvent[*corev1.Node]) bool { return false },
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on Node: %w", err)
	}

	return nil
}

// dialogueChanged reports whether the node changed in a way the dialogue depends on.
func dialogueChanged(oldNode, newNode *corev1.Node) bool {
	if oldNode == nil || newNode == nil {
		return true
	}

	for _, annotation := range []string{annotations.AnnNodeVMRestartRequired, annotations.AnnNodeVMRestartApproved} {
		_, oldFound := oldNode.GetAnnotations()[annotation]
		_, newFound := newNode.GetAnnotations()[annotation]
		if oldFound != newFound {
			return true
		}
	}

	return oldNode.Spec.Unschedulable != newNode.Spec.Unschedulable
}
