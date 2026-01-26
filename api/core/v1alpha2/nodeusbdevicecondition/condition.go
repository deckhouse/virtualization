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

package nodeusbdevicecondition

// Type represents the various condition types for the `NodeUSBDevice`.
type Type string

const (
	// AssignedType indicates whether a namespace is assigned for the device.
	AssignedType Type = "Assigned"
	// ReadyType indicates whether the device is ready to use.
	ReadyType Type = "Ready"
)

func (t Type) String() string {
	return string(t)
}

type (
	// AssignedReason represents the various reasons for the `Assigned` condition type.
	AssignedReason string
	// ReadyReason represents the various reasons for the `Ready` condition type.
	ReadyReason string
)

const (
	// Assigned signifies that namespace is assigned for the device and corresponding USBDevice resource is created in this namespace.
	Assigned AssignedReason = "Assigned"
	// Available signifies that no namespace is assigned for the device.
	Available AssignedReason = "Available"
	// InProgress signifies that device connection to namespace is in progress (USBDevice resource creation).
	InProgress AssignedReason = "InProgress"

	// Ready signifies that device is ready to use.
	Ready ReadyReason = "Ready"
	// NotReady signifies that device exists in the system but is not ready to use.
	NotReady ReadyReason = "NotReady"
	// NotFound signifies that device is absent on the host.
	NotFound ReadyReason = "NotFound"
)

func (r AssignedReason) String() string {
	return string(r)
}

func (r ReadyReason) String() string {
	return string(r)
}
