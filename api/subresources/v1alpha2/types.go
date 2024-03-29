package v1alpha2

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
