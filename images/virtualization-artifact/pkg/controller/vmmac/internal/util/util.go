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

package util

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/ip"
	"github.com/deckhouse/virtualization-controller/pkg/common/mac"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func GetAllocatedMACs(ctx context.Context, apiClient client.Client) (mac.AllocatedMACs, error) {
	var leases virtv2.VirtualMachineMACAddressLeaseList

	err := apiClient.List(ctx, &leases)
	if err != nil {
		return nil, fmt.Errorf("error getting leases: %w", err)
	}

	allocatedIPs := make(mac.AllocatedMACs, len(leases.Items))
	for _, lease := range leases.Items {
		l := lease
		allocatedIPs[ip.LeaseNameToIP(lease.Name)] = &l
	}

	return allocatedIPs, nil
}

func IsBoundLease(lease *virtv2.VirtualMachineMACAddressLease, vmmac *virtv2.VirtualMachineMACAddress) bool {
	if lease.Status.Phase != virtv2.VirtualMachineMACAddressLeasePhaseBound {
		return false
	}

	if lease.Spec.VirtualMachineMACAddressRef == nil {
		return false
	}

	if lease.Spec.VirtualMachineMACAddressRef.Namespace != vmmac.Namespace || lease.Spec.VirtualMachineMACAddressRef.Name != vmmac.Name {
		return false
	}

	return true
}
