package v2alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type VirtualMachineImage struct {
	metav1.TypeMeta `json:",inline"`

	metav1.ObjectMeta `json:"metadata,omitempty"`
}
