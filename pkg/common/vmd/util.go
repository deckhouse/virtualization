package cvmi

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	virtv2alpha1 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
)

// MakeOwnerReference makes owner reference from a ClusterVirtualMachineImage.
func MakeOwnerReference(vmd *virtv2alpha1.VirtualMachineDisk) metav1.OwnerReference {
	return *metav1.NewControllerRef(vmd, schema.GroupVersionKind{
		Group:   virtv2alpha1.APIGroup,
		Version: virtv2alpha1.APIVersion,
		Kind:    virtv2alpha1.VMDKind,
	})
}

func HasCABundle(vmd *virtv2alpha1.VirtualMachineDisk) bool {
	if vmd != nil &&
		vmd.Spec.DataSource != nil &&
		vmd.Spec.DataSource.Type == virtv2alpha1.DataSourceTypeHTTP &&
		vmd.Spec.DataSource.HTTP != nil {
		return len(vmd.Spec.DataSource.HTTP.CABundle) > 0
	}
	return false
}

func GetCABundle(vmd *virtv2alpha1.VirtualMachineDisk) string {
	if HasCABundle(vmd) {
		return string(vmd.Spec.DataSource.HTTP.CABundle)
	}
	return ""
}
