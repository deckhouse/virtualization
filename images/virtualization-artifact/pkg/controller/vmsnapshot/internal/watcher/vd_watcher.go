/*
Copyright 2024 Flant JSC

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
	"strings"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type VirtualDiskWatcher struct {
	client client.Client
}

func NewVirtualDiskWatcher(client client.Client) *VirtualDiskWatcher {
	return &VirtualDiskWatcher{
		client: client,
	}
}

func (w VirtualDiskWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.VirtualDisk{},
			handler.TypedEnqueueRequestsFromMapFunc(w.enqueueRequests),
			predicate.TypedFuncs[*virtv2.VirtualDisk]{
				CreateFunc: func(e event.TypedCreateEvent[*virtv2.VirtualDisk]) bool { return false },
				DeleteFunc: func(e event.TypedDeleteEvent[*virtv2.VirtualDisk]) bool { return false },
				UpdateFunc: func(e event.TypedUpdateEvent[*virtv2.VirtualDisk]) bool {
					if e.ObjectOld.Status.Phase != e.ObjectNew.Status.Phase {
						return true
					}

					oldInUseCondition, _ := conditions.GetCondition(vdcondition.InUseType, e.ObjectOld.Status.Conditions)
					newInUseCondition, _ := conditions.GetCondition(vdcondition.InUseType, e.ObjectNew.Status.Conditions)

					if oldInUseCondition != newInUseCondition {
						return true
					}

					oldResized, _ := conditions.GetCondition(vdcondition.ResizingType, e.ObjectOld.Status.Conditions)
					newResized, _ := conditions.GetCondition(vdcondition.ResizingType, e.ObjectNew.Status.Conditions)

					return oldResized.Status != newResized.Status || oldResized.Reason != newResized.Reason
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualDisk: %w", err)
	}
	return nil
}

func (w VirtualDiskWatcher) enqueueRequests(ctx context.Context, vd *virtv2.VirtualDisk) (requests []reconcile.Request) {
	var vmSnapshots virtv2.VirtualMachineSnapshotList
	err := w.client.List(ctx, &vmSnapshots, &client.ListOptions{
		Namespace: vd.GetNamespace(),
	})
	if err != nil {
		slog.Default().Error(fmt.Sprintf("failed to list virtual machine snapshots: %s", err))
		return
	}

	for _, vmSnapshot := range vmSnapshots.Items {
		for _, vdSnapshotName := range vmSnapshot.Status.VirtualDiskSnapshotNames {
			vdName, ok := strings.CutSuffix(vdSnapshotName, "-"+string(vmSnapshot.UID))
			if !ok {
				slog.Default().Error("Failed to get virtual disk name from virtual disk snapshot name, please report a bug")
				continue
			}

			if vdName == vd.GetName() {
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      vmSnapshot.Name,
						Namespace: vmSnapshot.Namespace,
					},
				})
				break
			}
		}
	}

	return
}
