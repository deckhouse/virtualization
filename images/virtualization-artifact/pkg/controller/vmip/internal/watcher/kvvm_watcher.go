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
	"strings"

	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func NewKVVMWatcher(client client.Client) *KVVMWatcher {
	return &KVVMWatcher{
		client: client,
		logger: log.Default().With("watcher", strings.ToLower(virtv1.VirtualMachineGroupVersionKind.Kind)),
	}
}

type KVVMWatcher struct {
	client client.Client
	logger *log.Logger
}

func (w *KVVMWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(
			mgr.GetCache(),
			&virtv1.VirtualMachine{},
			handler.TypedEnqueueRequestsFromMapFunc(w.enqueueRequests),
			predicate.TypedFuncs[*virtv1.VirtualMachine]{
				CreateFunc: func(tce event.TypedCreateEvent[*virtv1.VirtualMachine]) bool { return false },
				UpdateFunc: func(tue event.TypedUpdateEvent[*virtv1.VirtualMachine]) bool { return false },
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualMachine: %w", err)
	}
	return nil
}

func (w KVVMWatcher) enqueueRequests(ctx context.Context, kvvm *virtv1.VirtualMachine) []reconcile.Request {
	var requests []reconcile.Request

	vmipNames := make(map[string]struct{})

	vmips := &v1alpha2.VirtualMachineIPAddressList{}
	err := w.client.List(ctx, vmips, client.InNamespace(kvvm.Namespace), &client.MatchingFields{
		indexer.IndexFieldVMIPByVM: kvvm.Name,
	})
	if err != nil {
		w.logger.Error(fmt.Sprintf("failed to list vmips: %s", err))
		return nil
	}

	for _, vmip := range vmips.Items {
		vmipNames[vmip.Name] = struct{}{}
	}

	for vmipName := range vmipNames {
		requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{
			Namespace: kvvm.Namespace,
			Name:      vmipName,
		}})
	}

	return requests
}
