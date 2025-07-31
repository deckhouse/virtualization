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

// +genclient
// +genclient:readonly
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:conversion-gen:explicit-from=net/url.Values

type VirtualMachineUsbRedir struct {
	metav1.TypeMeta `json:",inline"`
}

// +genclient
// +genclient:readonly
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:conversion-gen:explicit-from=net/url.Values

type VirtualMachineConsole struct {
	metav1.TypeMeta `json:",inline"`
}

// +genclient
// +genclient:readonly
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:conversion-gen:explicit-from=net/url.Values

type VirtualMachineVNC struct {
	metav1.TypeMeta `json:",inline"`
}

// +genclient
// +genclient:readonly
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:conversion-gen:explicit-from=net/url.Values

type VirtualMachinePortForward struct {
	metav1.TypeMeta `json:",inline"`

	Protocol string `json:"protocol"`
	Port     int    `json:"port"`
}

// +genclient
// +genclient:readonly
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

// +genclient
// +genclient:readonly
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:conversion-gen:explicit-from=net/url.Values

type VirtualMachineRemoveVolume struct {
	metav1.TypeMeta `json:",inline"`
	Name            string `json:"name"`
}

// +genclient
// +genclient:readonly
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:conversion-gen:explicit-from=net/url.Values

type VirtualMachineFreeze struct {
	metav1.TypeMeta `json:",inline"`

	UnfreezeTimeout *metav1.Duration `json:"unfreezeTimeout"`
}

// +genclient
// +genclient:readonly
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:conversion-gen:explicit-from=net/url.Values

type VirtualMachineUnfreeze struct {
	metav1.TypeMeta `json:",inline"`
}

// +genclient
// +genclient:readonly
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:conversion-gen:explicit-from=net/url.Values

type VirtualMachineCancelEvacuation struct {
	metav1.TypeMeta

	DryRun []string `json:"dryRun,omitempty"`
}
