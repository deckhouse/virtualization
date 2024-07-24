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

	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func GetAllocatedIPs(ctx context.Context, apiClient client.Client, vmipType virtv2.VirtualMachineIPAddressType) (common.AllocatedIPs, error) {
	var leases virtv2.VirtualMachineIPAddressLeaseList

	err := apiClient.List(ctx, &leases)
	if err != nil {
		return nil, fmt.Errorf("error getting leases: %w", err)
	}

	allocatedIPs := make(common.AllocatedIPs, len(leases.Items))
	for _, lease := range leases.Items {
		l := lease
		if vmipType == virtv2.VirtualMachineIPAddressTypeStatic && l.Status.Phase == virtv2.VirtualMachineIPAddressLeasePhaseReleased {
			continue
		} else {
			allocatedIPs[common.LeaseNameToIP(lease.Name)] = &l
		}
	}

	return allocatedIPs, nil
}

func IsBoundLease(lease *virtv2.VirtualMachineIPAddressLease, vmip *virtv2.VirtualMachineIPAddress) bool {
	if lease.Status.Phase != virtv2.VirtualMachineIPAddressLeasePhaseBound {
		return false
	}

	if lease.Spec.VirtualMachineIPAddressRef == nil {
		return false
	}

	if lease.Spec.VirtualMachineIPAddressRef.Namespace != vmip.Namespace || lease.Spec.VirtualMachineIPAddressRef.Name != vmip.Name {
		return false
	}

	return true
}
