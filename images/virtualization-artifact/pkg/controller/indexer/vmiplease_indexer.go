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

package indexer

import (
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func IndexVMIPLeaseByVMIP() (obj client.Object, field string, extractValue client.IndexerFunc) {
	return &v1alpha2.VirtualMachineIPAddressLease{}, IndexFieldVMIPLeaseByVMIP, func(object client.Object) []string {
		lease, ok := object.(*v1alpha2.VirtualMachineIPAddressLease)
		if !ok || lease == nil {
			return nil
		}
		vmipRef := lease.Spec.VirtualMachineIPAddressRef
		if vmipRef == nil || vmipRef.Name == "" {
			return nil
		}

		return []string{fmt.Sprintf("%s/%s", vmipRef.Namespace, vmipRef.Name)}
	}
}
