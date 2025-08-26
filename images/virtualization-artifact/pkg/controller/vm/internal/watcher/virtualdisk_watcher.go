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
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

func NewVirtualDiskWatcher() *VirtualDiskWatcher {
	return &VirtualDiskWatcher{}
}

type VirtualDiskWatcher struct{}

func (w *VirtualDiskWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(
			mgr.GetCache(),
			&virtv2.VirtualDisk{},
			handler.TypedEnqueueRequestsFromMapFunc(enqueueRequestsBlockDevice[*virtv2.VirtualDisk](mgr.GetClient())),
			predicate.TypedFuncs[*virtv2.VirtualDisk]{
				UpdateFunc: func(e event.TypedUpdateEvent[*virtv2.VirtualDisk]) bool {
					if e.ObjectOld.Status.Phase != e.ObjectNew.Status.Phase {
						return true
					}

					oldInUseCondition, _ := conditions.GetCondition(vdcondition.InUseType, e.ObjectOld.Status.Conditions)
					newInUseCondition, _ := conditions.GetCondition(vdcondition.InUseType, e.ObjectNew.Status.Conditions)
					if !equality.Semantic.DeepEqual(oldInUseCondition, newInUseCondition) {
						return true
					}

					if oldInUseCondition != newInUseCondition {
						return true
					}

					if e.ObjectOld.Status.Target.PersistentVolumeClaim != e.ObjectNew.Status.Target.PersistentVolumeClaim {
						return true
					}

					oldMigrationCondition, _ := conditions.GetCondition(vdcondition.MigratingType, e.ObjectOld.Status.Conditions)
					newMigrationCondition, _ := conditions.GetCondition(vdcondition.MigratingType, e.ObjectNew.Status.Conditions)
					if !equality.Semantic.DeepEqual(oldMigrationCondition, newMigrationCondition) {
						return true
					}

					return !equality.Semantic.DeepEqual(e.ObjectOld.Status.MigrationState, e.ObjectNew.Status.MigrationState)
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualDisk: %w", err)
	}
	return nil
}
