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
	"strings"

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// VMContainerNameSuffix - a name suffix for container with virt-launcher, libvirt and qemu processes.
// Container name is "d8v-compute", but previous versions may have "compute" container.
const VMContainerNameSuffix = "compute"

// CalculateCoresAndSockets calculates the number of sockets and cores per socket needed to achieve
// the desired total number of CPU cores.
// The function tries to minimize the number of sockets while ensuring the desired core count.
//
// https://bugzilla.redhat.com/show_bug.cgi?id=1653453
// The number of cores per socket and the growth of the number of sockets is chosen in such a way as
// to have less impact on the performance of the virtual machine, as well as on compatibility with operating systems
func CalculateCoresAndSockets(desiredCores int) (sockets, coresPerSocket int) {
	if desiredCores < 1 {
		return 1, 1
	}

	if desiredCores <= 16 {
		return 1, desiredCores
	}

	switch {
	case desiredCores <= 32:
		sockets = 2
	case desiredCores <= 64:
		sockets = 4
	default:
		sockets = 8
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

func IsComputeContainer(name string) bool {
	return strings.HasSuffix(name, VMContainerNameSuffix)
}
