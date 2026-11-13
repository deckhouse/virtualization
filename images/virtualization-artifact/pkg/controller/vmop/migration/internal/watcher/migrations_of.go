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

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonvmop "github.com/deckhouse/virtualization-controller/pkg/common/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// migrationsOf returns the requests for the unfinished migrations of the virtual machine vmName.
func migrationsOf(ctx context.Context, c client.Client, namespace, vmName string) []reconcile.Request {
	vmops := &v1alpha2.VirtualMachineOperationList{}
	if err := c.List(ctx, vmops,
		client.InNamespace(namespace),
		client.MatchingFields{indexer.IndexFieldVMOPByVM: vmName},
	); err != nil {
		return nil
	}

	var requests []reconcile.Request

	for i := range vmops.Items {
		vmop := &vmops.Items[i]
		if !Match(vmop) || commonvmop.IsFinished(vmop) {
			continue
		}

		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{Namespace: vmop.GetNamespace(), Name: vmop.GetName()},
		})
	}

	return requests
}
