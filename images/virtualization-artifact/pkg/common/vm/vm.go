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

import (
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	// SocketsForUpTo32Cores represents the number of sockets required for configurations
	// with up to 32 cores in total.
	SocketsForUpTo32Cores = 2

	// SocketsForUpTo64Cores represents the number of sockets required for configurations
	// with up to 64 cores in total.
	SocketsForUpTo64Cores = 4

	// SocketsForMoreThan64Cores represents the number of sockets required for configurations
	// with more than 64 cores in total.
	SocketsForMoreThan64Cores = 8

	// MaxCoresPerSocket defines the maximum number of cores that can be allocated to a single socket.
	MaxCoresPerSocket = 16

	// MaxCoresFor2Sockets defines the maximum number of cores that can be allocated across two sockets.
	MaxCoresFor2Sockets = MaxCoresPerSocket * 2

	// MaxCoresFor4Sockets defines the maximum number of cores that can be allocated across four sockets.
	MaxCoresFor4Sockets = MaxCoresPerSocket * 4
)

// CalculateCoresAndSockets calculates the number of sockets and cores per socket needed to achieve
// the desired total number of CPU cores.
// The function tries to minimize the number of sockets while ensuring the desired core count.
func CalculateCoresAndSockets(desiredCores int) (sockets, coresPerSocket int) {
	if desiredCores < 1 {
		return 1, 1
	}

	if desiredCores <= MaxCoresPerSocket {
		return 1, desiredCores
	}

	switch {
	case desiredCores <= MaxCoresFor2Sockets:
		sockets = SocketsForUpTo32Cores
	case desiredCores <= MaxCoresFor4Sockets:
		sockets = SocketsForUpTo64Cores
	default:
		sockets = SocketsForMoreThan64Cores
	}

	coresPerSocket = desiredCores / sockets
	if desiredCores%sockets != 0 {
		coresPerSocket++
	}

	return sockets, coresPerSocket
}

func ApprovalMode(vm *virtv2.VirtualMachine) virtv2.RestartApprovalMode {
	if vm.Spec.Disruptions == nil {
		return virtv2.Manual
	}
	return vm.Spec.Disruptions.RestartApprovalMode
}
