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

package vmclasscondition

type Type string

func (t Type) String() string {
	return string(t)
}

const (
	TypeReady      Type = "Ready"
	TypeDiscovered Type = "Discovered"
	TypeInUse      Type = "InUse"
)

type Reason string

func (r Reason) String() string {
	return string(r)
}

const (
	// ReasonNoCpuFeaturesEnabled determines that processor functions are not available.
	ReasonNoCpuFeaturesEnabled Reason = "NoCpuFeaturesEnabled"
	// ReasonNoSuitableNodesFound determines that no suitable node has been found.
	ReasonNoSuitableNodesFound Reason = "NoSuitableNodesFound"
	// ReasonSuitableNodesFound determines that suitable node has been found.
	ReasonSuitableNodesFound Reason = "SuitableNodesFound"

	ReasonDiscoverySucceeded Reason = "DiscoverySucceeded"
	ReasonDiscoverySkip      Reason = "DiscoverySkip"
	ReasonDiscoveryFailed    Reason = "DiscoveryFailed"

	// ReasonVMClassInUse is the event reason indicating that the VMClass is being used by a virtual machine.
	ReasonVMClassInUse Reason = "VirtualMachineClassInUse"
)
