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

package vm

import virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"

func ApprovalMode(vm *virtv2.VirtualMachine) virtv2.RestartApprovalMode {
	if vm.Spec.Disruptions == nil {
		return virtv2.Manual
	}
	return vm.Spec.Disruptions.RestartApprovalMode
}

// CalculateSockets calculates the number of CPU sockets needed based on the number of cores.
// It returns:
// - 1 socket for up to 16 cores
// - 2 sockets for 17 to 32 cores
// - 4 sockets for 33 to 64 cores
// - 8 sockets for more than 64 cores
func CalculateSockets(cores int) int {
	switch {
	case cores <= 16:
		return 1
	case cores >= 17 && cores <= 32:
		return 2
	case cores >= 33 && cores <= 64:
		return 4
	case cores >= 65:
		return 8
	default:
		return 1
	}
}
