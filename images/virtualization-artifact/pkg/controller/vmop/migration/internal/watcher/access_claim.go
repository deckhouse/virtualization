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

	"k8s.io/apimachinery/pkg/api/equality"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	coordinationv1alpha1 "github.com/deckhouse/virtualization/api/coordination/v1alpha1"
)

func NewAccessClaimWatcher() *AccessClaimWatcher {
	return &AccessClaimWatcher{}
}

// AccessClaimWatcher wakes up the migrations of a virtual machine that wait for its snapshot once the
// session of the snapshot's access claim moves on: its phase changes, the guest is unfrozen, or the claim
// goes away.
type AccessClaimWatcher struct{}

func (w AccessClaimWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	mgrClient := mgr.GetClient()
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &coordinationv1alpha1.VirtualMachineAccessClaim{},
			handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context, claim *coordinationv1alpha1.VirtualMachineAccessClaim) []reconcile.Request {
				return migrationsOf(ctx, mgrClient, claim.GetNamespace(), claim.Spec.VirtualMachine.Name)
			}),
			predicate.TypedFuncs[*coordinationv1alpha1.VirtualMachineAccessClaim]{
				CreateFunc: func(e event.TypedCreateEvent[*coordinationv1alpha1.VirtualMachineAccessClaim]) bool { return false },
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
