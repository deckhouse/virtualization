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
		source.Kind(
			mgr.GetCache(),
			&virtv2.VirtualImage{},
			handler.TypedEnqueueRequestsFromMapFunc(enqueueRequestsBlockDevice[*virtv2.VirtualImage](mgr.GetClient())),
			predicate.TypedFuncs[*virtv2.VirtualImage]{
				UpdateFunc: func(e event.TypedUpdateEvent[*virtv2.VirtualImage]) bool {
					return e.ObjectOld.Status.Phase != e.ObjectNew.Status.Phase
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualImage: %w", err)
	}
	return nil
}

func enqueueRequestsBlockDevice[T client.Object](cl client.Client) func(ctx context.Context, obj T) []reconcile.Request {
	return func(ctx context.Context, obj T) []reconcile.Request {
		var opts []client.ListOption
		switch obj.GetObjectKind().GroupVersionKind().Kind {
		case virtv2.VirtualImageKind:
			opts = append(opts,
				client.InNamespace(obj.GetNamespace()),
				client.MatchingFields{indexer.IndexFieldVMByVI: obj.GetName()},
			)
		case virtv2.ClusterVirtualImageKind:
			opts = append(opts,
				client.MatchingFields{indexer.IndexFieldVMByCVI: obj.GetName()},
			)
		case virtv2.VirtualDiskKind:
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
