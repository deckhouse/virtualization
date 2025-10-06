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
	"fmt"

	"k8s.io/apimachinery/pkg/api/equality"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func NewKVVMWatcher() *KVVMWatcher {
	return &KVVMWatcher{}
}

type KVVMWatcher struct{}

func (w *KVVMWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(
			mgr.GetCache(),
			&virtv1.VirtualMachine{},
			handler.TypedEnqueueRequestForOwner[*virtv1.VirtualMachine](
				mgr.GetScheme(),
				mgr.GetRESTMapper(),
				&v1alpha2.VirtualMachine{},
				handler.OnlyControllerOwner(),
			),
			predicate.TypedFuncs[*virtv1.VirtualMachine]{
				UpdateFunc: func(e event.TypedUpdateEvent[*virtv1.VirtualMachine]) bool {
					oldVM := e.ObjectOld
					newVM := e.ObjectNew
					oldSynchronizedCondition, _ := conditions.GetKVVMCondition(conditions.VirtualMachineSynchronized, oldVM.Status.Conditions)
					newSynchronizedCondition, _ := conditions.GetKVVMCondition(conditions.VirtualMachineSynchronized, newVM.Status.Conditions)

					return oldVM.Status.PrintableStatus != newVM.Status.PrintableStatus ||
						oldSynchronizedCondition.Status != newSynchronizedCondition.Status ||
						oldSynchronizedCondition.Reason != newSynchronizedCondition.Reason ||
						oldVM.Status.Ready != newVM.Status.Ready ||
						oldVM.Annotations[annotations.AnnVMStartRequested] != newVM.Annotations[annotations.AnnVMStartRequested] ||
						oldVM.Annotations[annotations.AnnVMRestartRequested] != newVM.Annotations[annotations.AnnVMRestartRequested] ||
						!equality.Semantic.DeepEqual(oldVM.Status.VolumeSnapshotStatuses, newVM.Status.VolumeSnapshotStatuses)
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualMachine: %w", err)
	}
	return nil
}
