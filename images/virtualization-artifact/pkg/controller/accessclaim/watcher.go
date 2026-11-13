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

package accessclaim

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/equality"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	coordinationv1alpha1 "github.com/deckhouse/virtualization/api/coordination/v1alpha1"
)

// Watcher wakes the snapshot owning a claim up when the coordinator decides on the claim or reports its
// progress.
type Watcher struct {
	owner client.Object
}

// NewWatcher returns a Watcher for the claims of snapshots of the owner's type.
func NewWatcher(owner client.Object) *Watcher {
	return &Watcher{owner: owner}
}

func (w Watcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &coordinationv1alpha1.VirtualMachineAccessClaim{},
			handler.TypedEnqueueRequestForOwner[*coordinationv1alpha1.VirtualMachineAccessClaim](
				mgr.GetScheme(),
				mgr.GetRESTMapper(),
				w.owner,
				handler.OnlyControllerOwner(),
			),
			predicate.TypedFuncs[*coordinationv1alpha1.VirtualMachineAccessClaim]{
				UpdateFunc: func(e event.TypedUpdateEvent[*coordinationv1alpha1.VirtualMachineAccessClaim]) bool {
					return e.ObjectOld.Status.Phase != e.ObjectNew.Status.Phase ||
						!equality.Semantic.DeepEqual(e.ObjectOld.Status.Conditions, e.ObjectNew.Status.Conditions)
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualMachineAccessClaim: %w", err)
	}

	return nil
}
