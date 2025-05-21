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

package vmipcondition

type Type string

const (
	// BoundType represents the condition type when a Virtual Machine IP is bound.
	BoundType Type = "Bound"

	// AttachedType represents the condition type when a Virtual Machine IP is attached.
	AttachedType Type = "Attached"
)

func (t Type) String() string {
	return string(t)
}

type (
	// BoundReason represents specific reasons for the 'Bound' condition type.
	BoundReason string

	// AttachedReason represents specific reasons for the 'Attached' condition type.
	AttachedReason string
)

func (r BoundReason) String() string {
	return string(r)
}

func (r AttachedReason) String() string {
	return string(r)
}

const (
	// VirtualMachineIPAddressIsOutOfTheValidRange is a BoundReason indicating when specified IP address is out of the range in controller settings.
	VirtualMachineIPAddressIsOutOfTheValidRange BoundReason = "VirtualMachineIPAddressIsOutOfTheValidRange"
	// VirtualMachineIPAddressLeaseAlreadyExists is a BoundReason indicating the IP address lease already exists.
	VirtualMachineIPAddressLeaseAlreadyExists BoundReason = "VirtualMachineIPAddressLeaseAlreadyExists"
	// VirtualMachineIPAddressLeaseLost is a BoundReason indicating the IP address lease was lost.
	VirtualMachineIPAddressLeaseLost BoundReason = "VirtualMachineIPAddressLeaseLost"
	// VirtualMachineIPAddressLeaseNotFound is a BoundReason indicating the IP address lease was not found.
	VirtualMachineIPAddressLeaseNotFound BoundReason = "VirtualMachineIPAddressLeaseNotFound"
	// VirtualMachineIPAddressLeaseNotReady is a BoundReason indicating the IP address lease was not ready.
	VirtualMachineIPAddressLeaseNotReady BoundReason = "VirtualMachineIPAddressLeaseNotReady"
	// Bound is a BoundReason indicating the IP address lease is successfully bound.
	Bound BoundReason = "Bound"

	// VirtualMachineNotFound is an AttachedReason indicating the Virtual Machine was not found.
	VirtualMachineNotFound AttachedReason = "VirtualMachineNotFound"
	// Attached is an AttachedReason indicating the IP address was successfully attached to the Virtual Machine.
	Attached AttachedReason = "Attached"
)
