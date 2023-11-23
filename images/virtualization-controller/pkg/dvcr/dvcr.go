package dvcr

import (
	"fmt"
	"path"

	virtv2alpha1 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
)

type Settings struct {
	// AuthSecret is a name of the Secret with docker authentication.
	AuthSecret string
	// AuthSecretNamespace is a namespace for the AuthSecret.
	AuthSecretNamespace string
	// CertsSecret is a name of the TLS Secret with DVCR certificates (only CA cert is used).
	CertsSecret string
	// CertsSecretNamespace is a namespace for the CertsSecret.
	CertsSecretNamespace string
	// RegistryURL is a registry hostname with optional endpoint.
	RegistryURL string
	// InsecureTLS specifies if registry is insecure (trust all certificates). Works for destination only.
	InsecureTLS string
}

const (
	CVMIImageTmpl = "cvmi/%s"
	VMIImageTmpl  = "vmi/%s/%s"
	VMDImageTmpl  = "vmd/%s/%s"
)

// ImagePathForCVMI returns image name for CVMI.
func ImagePathForCVMI(cvmi *virtv2alpha1.ClusterVirtualMachineImage) string {
	ep := fmt.Sprintf(CVMIImageTmpl, cvmi.Name)
	return path.Clean(ep)
}

// ImagePathForVMI returns image name for VMI.
func ImagePathForVMI(vmi *virtv2alpha1.VirtualMachineImage) string {
	ep := fmt.Sprintf(VMIImageTmpl, vmi.Namespace, vmi.Name)
	return path.Clean(ep)
}

// ImagePathForVMD returns image name for VMD.
func ImagePathForVMD(vmd *virtv2alpha1.VirtualMachineDisk) string {
	ep := fmt.Sprintf(VMDImageTmpl, vmd.Namespace, vmd.Name)
	return path.Clean(ep)
}

// RegistryImageName prepares full registry URL for image path to use by dvcr-importer, dvcr-uploader, and DataVolume.
func RegistryImageName(dvcr *Settings, imagePath string) string {
	return path.Join(dvcr.RegistryURL, imagePath)
}
