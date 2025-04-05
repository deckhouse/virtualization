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

package indexer

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func IndexVMIPLeaseByVMIP(ctx context.Context, mgr manager.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &virtv2.VirtualMachineIPAddressLease{}, IndexFieldVMIPLeaseByVMIP, func(object client.Object) []string {
		lease, ok := object.(*virtv2.VirtualMachineIPAddressLease)
		if !ok || lease == nil {
			return nil
		}
		return []string{lease.Spec.VirtualMachineIPAddressRef.Name}
	})
}

func IndexVMMACLeaseByVMMAC(ctx context.Context, mgr manager.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &virtv2.VirtualMachineMACAddressLease{}, IndexFieldVMMACLeaseByVMMAC, func(object client.Object) []string {
		lease, ok := object.(*virtv2.VirtualMachineMACAddressLease)
		if !ok || lease == nil {
			return nil
		}
		return []string{lease.Spec.VirtualMachineMACAddressRef.Name}
	})
}
