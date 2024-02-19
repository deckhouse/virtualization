package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	VirtualMachineConsoleKind     = "VirtualMachineConsole"
	VirtualMachineConsoleResource = "virtualmachineconsoles"
)

// VirtualMachineConsole is a sub-resource for connecting to VirtualMachine using the console.
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineConsole struct {
	metav1.TypeMeta `json:",inline"`
}
