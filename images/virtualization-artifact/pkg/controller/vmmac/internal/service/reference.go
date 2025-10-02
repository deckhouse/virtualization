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
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func HasReference(vmmac *v1alpha2.VirtualMachineMACAddress, lease *v1alpha2.VirtualMachineMACAddressLease) bool {
	if vmmac == nil || lease == nil {
		return false
	}

	vmmacRef := lease.Spec.VirtualMachineMACAddressRef

	return vmmacRef != nil && vmmacRef.Name == vmmac.Name && vmmacRef.Namespace == vmmac.Namespace
}
