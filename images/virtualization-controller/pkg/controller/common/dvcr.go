package common

import (
	"fmt"
	"path"

	virtv2alpha1 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
)

type DVCRSettings struct {
	AuthSecret    string
	Registry      string
	InsecureTLS   string
	RegistryForVM string
}

const (
	CVMIImageTmpl = "cvmi/%s"
	VMIImageTmpl  = "vmi/%s/%s"
	VMDImageTmpl  = "vmd/%s/%s"
)

func NewDVCRSettings(authSecret, registry, registryForVM, insecureTLS string) *DVCRSettings {
	return &DVCRSettings{
		AuthSecret:    authSecret,
		Registry:      registry,
		RegistryForVM: registryForVM,
		InsecureTLS:   insecureTLS,
	}
}

// DVCRImageNameFromCVMI returns image name for CVMI.
func DVCRImageNameFromCVMI(cvmi *virtv2alpha1.ClusterVirtualMachineImage) string {
	ep := fmt.Sprintf(CVMIImageTmpl, cvmi.Name)
	return path.Clean(ep)
}

// DVCRImageNameFromVMI returns image name for VMI.
func DVCRImageNameFromVMI(vmi *virtv2alpha1.VirtualMachineImage) string {
	ep := fmt.Sprintf(VMIImageTmpl, vmi.Namespace, vmi.Name)
	return path.Clean(ep)
}

// DVCRImageNameFromVMD returns image name for VMD.
func DVCRImageNameFromVMD(vmd *virtv2alpha1.VirtualMachineDisk) string {
	ep := fmt.Sprintf(VMDImageTmpl, vmd.Namespace, vmd.Name)
	return path.Clean(ep)
}

// DVCREndpointForImporter prepares endpoint to use by dvcr-importer, dvcr-uploader, and DataVolume.
func DVCREndpointForImporter(dvcr *DVCRSettings, imageName string) string {
	return path.Join(dvcr.Registry, imageName)
}

// DVCREndpointForVM prepares endpoint to use in containerDisk in VirtualMachines.
// It uses dvcr registry or a separate vm registry if specified in RegistryForVm field.
// E.g. to use internal name for importer Pods and publicly available domain for kubelet.
func DVCREndpointForVM(dvcr *DVCRSettings, imageName string) string {
	registry := dvcr.RegistryForVM
	if registry == "" {
		registry = dvcr.Registry
	}
	return path.Join(registry, imageName)
}
