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

package v1alpha2

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type VirtualMachine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:conversion-gen:explicit-from=net/url.Values

type VirtualMachineConsole struct {
	metav1.TypeMeta `json:",inline"`

	// Probe asks who holds the serial console instead of connecting to it: the answer is a
	// VirtualMachineSession and the session of the current holder is left alone. Connecting is
	// exclusive, so a client is expected to probe first and warn the user about who is going to
	// be disconnected.
	Probe bool `json:"probe,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:conversion-gen:explicit-from=net/url.Values

type VirtualMachineVNC struct {
	metav1.TypeMeta `json:",inline"`

	// Probe asks who holds the VNC instead of connecting to it. See VirtualMachineConsole.Probe.
	Probe bool `json:"probe,omitempty"`
}

// VirtualMachineSession reports who is currently connected to the serial console or to the VNC
// of the virtual machine, depending on the subresource it is read from. Both are exclusive:
// connecting disconnects that user, so clients are expected to read this first and warn before
// they do.
//
// This is a plain struct rather than an API object: console and vnc are subresources with a raw
// upgradable connection, which is why the server can write arbitrary data into that connection to
// talk to the client before the connection is upgraded. A probe request is answered exactly there,
// as JSON written into the not-yet-upgraded connection — it never goes through the scheme, the
// conversion or the codecs, so the type needs no TypeMeta, no deepcopy and no registration.
type VirtualMachineSession struct {
	// Holder is the connected user. Empty if nobody is connected.
	Holder string `json:"holder,omitempty"`
	// StartTime is when that user connected.
	StartTime *metav1.Time `json:"startTime,omitempty"`
	// Client is how the connected client introduced itself, its user agent. It tells a UI from
	// a command line client, which often is what one needs to go and find the person.
	Client string `json:"client,omitempty"`
}

// SessionKind is the exclusive stream a session is held on.
type SessionKind string

const (
	ConsoleSession SessionKind = "console"
	VNCSession     SessionKind = "vnc"
)

// Description is the user-facing name of the stream, as it goes into a warning or an event.
func (k SessionKind) Description() string {
	switch k {
	case ConsoleSession:
		return "serial console"
	case VNCSession:
		return "VNC"
	default:
		return string(k)
	}
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:conversion-gen:explicit-from=net/url.Values

type VirtualMachinePortForward struct {
	metav1.TypeMeta `json:",inline"`

	Protocol string `json:"protocol"`
	Port     int    `json:"port"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:conversion-gen:explicit-from=net/url.Values

type VirtualMachineAddVolume struct {
	metav1.TypeMeta `json:",inline"`
	Name            string `json:"name"`
	VolumeKind      string `json:"volumeKind"`
	PVCName         string `json:"pvcName"`
	Image           string `json:"image"`
	Serial          string `json:"serial"`
	IsCdrom         bool   `json:"isCdrom"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:conversion-gen:explicit-from=net/url.Values

type VirtualMachineRemoveVolume struct {
	metav1.TypeMeta `json:",inline"`
	Name            string `json:"name"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:conversion-gen:explicit-from=net/url.Values

type VirtualMachineFreeze struct {
	metav1.TypeMeta `json:",inline"`

	UnfreezeTimeout *metav1.Duration `json:"unfreezeTimeout"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:conversion-gen:explicit-from=net/url.Values

type VirtualMachineUnfreeze struct {
	metav1.TypeMeta `json:",inline"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:conversion-gen:explicit-from=net/url.Values

type VirtualMachineCancelEvacuation struct {
	metav1.TypeMeta `json:",inline"`

	DryRun []string `json:"dryRun,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:conversion-gen:explicit-from=net/url.Values

type VirtualMachineAddResourceClaim struct {
	metav1.TypeMeta `json:",inline"`

	Name                      string `json:"name"`
	ResourceClaimTemplateName string `json:"resourceClaimTemplateName"`
	RequestName               string `json:"requestName"`

	DryRun []string `json:"dryRun,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:conversion-gen:explicit-from=net/url.Values

type VirtualMachineRemoveResourceClaim struct {
	metav1.TypeMeta

	Name   string   `json:"name"`
	DryRun []string `json:"dryRun,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type VirtualMachinePool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:conversion-gen:explicit-from=net/url.Values

type VirtualMachinePoolScaleDownWith struct {
	metav1.TypeMeta `json:",inline"`

	// Targets are the names of the pool member VirtualMachines to remove.
	Targets []string `json:"targets"`

	DryRun []string `json:"dryRun,omitempty"`
}
