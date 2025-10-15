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
	"log/slog"
	"strings"

	"k8s.io/apimachinery/pkg/fields"
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
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func NewVirtualImageWatcher(client client.Client) *VirtualImageWatcher {
	return &VirtualImageWatcher{
		client: client,
		logger: slog.Default().With("watcher", strings.ToLower(v1alpha2.VirtualImageKind)),
	}
}

type VirtualImageWatcher struct {
	client client.Client
	logger *slog.Logger
}

func (w *VirtualImageWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(
			mgr.GetCache(),
			&v1alpha2.VirtualImage{},
			handler.TypedEnqueueRequestsFromMapFunc(w.enqueue),
			predicate.TypedFuncs[*v1alpha2.VirtualImage]{
				UpdateFunc: func(e event.TypedUpdateEvent[*v1alpha2.VirtualImage]) bool {
					return e.ObjectOld.Status.Phase != e.ObjectNew.Status.Phase
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualImage: %w", err)
	}
	return nil
}

func (w *VirtualImageWatcher) enqueue(ctx context.Context, vi *v1alpha2.VirtualImage) []reconcile.Request {
	var vms v1alpha2.VirtualMachineList
	err := w.client.List(ctx, &vms, &client.ListOptions{
		Namespace:     vi.Namespace,
		FieldSelector: fields.OneTermEqualSelector(indexer.IndexFieldVMByVI, vi.Name),
	})
	if err != nil {
		w.logger.Error(fmt.Sprintf("failed to list virtual machines: %v", err))
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
