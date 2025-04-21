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
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func NewVirtualImageWatcher() *VirtualImageWatcher {
	return &VirtualImageWatcher{}
}

type VirtualImageWatcher struct{}

func (w *VirtualImageWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.VirtualImage{}),
		handler.EnqueueRequestsFromMapFunc(enqueueRequestsBlockDevice(mgr.GetClient(), virtv2.ImageDevice)),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool {
				oldVi, oldOk := e.ObjectOld.(*virtv2.VirtualImage)
				newVi, newOk := e.ObjectNew.(*virtv2.VirtualImage)
				if !oldOk || !newOk {
					return false
				}
				return oldVi.Status.Phase != newVi.Status.Phase
			},
		},
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualImage: %w", err)
	}
	return nil
}

func enqueueRequestsBlockDevice(cl client.Client, kind virtv2.BlockDeviceKind) func(ctx context.Context, obj client.Object) []reconcile.Request {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		var opts []client.ListOption
		switch kind {
		case virtv2.ImageDevice:
			if _, ok := obj.(*virtv2.VirtualImage); !ok {
				return nil
			}
			opts = append(opts,
				client.InNamespace(obj.GetNamespace()),
				client.MatchingFields{indexer.IndexFieldVMByVI: obj.GetName()},
			)
		case virtv2.ClusterImageDevice:
			if _, ok := obj.(*virtv2.ClusterVirtualImage); !ok {
				return nil
			}
			opts = append(opts,
				client.MatchingFields{indexer.IndexFieldVMByCVI: obj.GetName()},
			)
		case virtv2.DiskDevice:
			if _, ok := obj.(*virtv2.VirtualDisk); !ok {
				return nil
			}
			opts = append(opts,
				client.InNamespace(obj.GetNamespace()),
				client.MatchingFields{indexer.IndexFieldVMByVD: obj.GetName()},
			)
		default:
			return nil
		}
		var vms virtv2.VirtualMachineList
		if err := cl.List(ctx, &vms, opts...); err != nil {
			return nil
		}
		var result []reconcile.Request
		for _, vm := range vms.Items {
			result = append(result, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      vm.GetName(),
					Namespace: vm.GetNamespace(),
				},
			})
		}
		return result
	}
}
