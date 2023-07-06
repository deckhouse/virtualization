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

func UpdateDVCREnvsFromCVMI(podEnvVars *ImportPodEnvVar, cvmi *virtv2alpha1.ClusterVirtualMachineImage, dvcr *DVCRSettings) {
	podEnvVars.DestinationAuthSecret = dvcr.AuthSecret
	podEnvVars.DestinationEndpoint = PrepareDVCREndpointFromCVMI(cvmi, dvcr)
	podEnvVars.DestinationInsecureTLS = dvcr.InsecureTLS
}

func UpdateDVCREnvsFromVMI(podEnvVars *ImportPodEnvVar, vmi *virtv2alpha1.VirtualMachineImage, dvcr *DVCRSettings) {
	podEnvVars.DestinationAuthSecret = dvcr.AuthSecret
	podEnvVars.DestinationEndpoint = PrepareDVCREndpointFromVMI(vmi, dvcr)
	podEnvVars.DestinationInsecureTLS = dvcr.InsecureTLS
}
