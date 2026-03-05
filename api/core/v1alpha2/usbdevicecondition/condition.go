/*
Copyright 2026 Flant JSC

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

package usbdevicecondition

// Type represents the various condition types for the `USBDevice`.
type Type string

const (
	// ReadyType indicates whether the device is ready to use.
	ReadyType Type = "Ready"
	// AttachedType indicates whether the device is attached to a virtual machine.
	AttachedType Type = "Attached"
)

func (t Type) String() string {
	return string(t)
}

type (
	// ReadyReason represents the various reasons for the `Ready` condition type.
	ReadyReason string
	// AttachedReason represents the various reasons for the `Attached` condition type.
	AttachedReason string
)

const (
	// Ready signifies that device is ready to use.
	Ready ReadyReason = "Ready"
	// NotReady signifies that device exists in the system but is not ready to use.
	NotReady ReadyReason = "NotReady"
	// NotFound signifies that device is absent on the host.
	NotFound ReadyReason = "NotFound"

	// AttachedToVirtualMachine signifies that device is attached to a virtual machine.
	AttachedToVirtualMachine AttachedReason = "AttachedToVirtualMachine"
	// Available signifies that device is available for attachment to a virtual machine.
	Available AttachedReason = "Available"
	// DetachedForMigration signifies that device was detached for migration (e.g. live migration).
	DetachedForMigration AttachedReason = "DetachedForMigration"
)

func (r ReadyReason) String() string {
	return string(r)
}

func (r AttachedReason) String() string {
	return string(r)
}
