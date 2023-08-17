package common

import (
	"fmt"
	"path"

	virtv2alpha1 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
)

type DVCRSettings struct {
	AuthSecret  string
	Registry    string
	InsecureTLS string
}

const (
	CVMIPath = "%s/cvmi/%s"
	VMIPath  = "%s/vmi/%s/%s"
	VMDPath  = "%s/vmd/%s/%s"
)

func NewDVCRSettings(authSecret, registry, insecureTLS string) *DVCRSettings {
	return &DVCRSettings{
		AuthSecret:  authSecret,
		Registry:    registry,
		InsecureTLS: insecureTLS,
	}
}

// PrepareDVCREndpointFromCVMI returns cvmi endpoint in registry.
func PrepareDVCREndpointFromCVMI(cvmi *virtv2alpha1.ClusterVirtualMachineImage, dvcr *DVCRSettings) string {
	ep := fmt.Sprintf(CVMIPath, dvcr.Registry, cvmi.Name)
	return path.Clean(ep)
}

// PrepareDVCREndpointFromVMI returns vmi endpoint in registry.
func PrepareDVCREndpointFromVMI(vmi *virtv2alpha1.VirtualMachineImage, dvcr *DVCRSettings) string {
	ep := fmt.Sprintf(VMIPath, dvcr.Registry, vmi.Namespace, vmi.Name)
	return path.Clean(ep)
}

// PrepareDVCREndpointFromVMD returns vmd endpoint in registry.
func PrepareDVCREndpointFromVMD(vmd *virtv2alpha1.VirtualMachineDisk, dvcr *DVCRSettings) string {
	ep := fmt.Sprintf(VMDPath, dvcr.Registry, vmd.Namespace, vmd.Name)
	return path.Clean(ep)
}
