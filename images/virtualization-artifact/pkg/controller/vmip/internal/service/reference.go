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

package service

import (
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func HasReference(vmip *virtv2.VirtualMachineIPAddress, lease *virtv2.VirtualMachineIPAddressLease) bool {
	if vmip == nil || lease == nil {
		return false
	}

	if lease.Labels[annotations.LabelVirtualMachineIPAddressUID] != string(vmip.GetUID()) {
		return false
	}

	vmipRef := lease.Spec.VirtualMachineIPAddressRef

	return vmipRef != nil && vmipRef.Name == vmip.Name && vmipRef.Namespace == vmip.Namespace
}
