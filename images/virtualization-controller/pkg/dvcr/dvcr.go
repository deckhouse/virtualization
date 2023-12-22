package dvcr

import (
	"fmt"
	"path"
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
	// UploaderIngressSettings are settings for uploading images to the DVCR using ingress.
	UploaderIngressSettings UploaderIngressSettings
}

type UploaderIngressSettings struct {
	Host               string
	TLSSecret          string
	TLSSecretNamespace string
	Class              string
}

const (
	CVMIImageTmpl = "cvmi/%s"
	VMIImageTmpl  = "vmi/%s/%s"
	VMDImageTmpl  = "vmd/%s/%s"
)

// RegistryImageForCVMI returns image name for CVMI.
func (s *Settings) RegistryImageForCVMI(name string) string {
	imgPath := path.Clean(fmt.Sprintf(CVMIImageTmpl, name))
	return path.Join(s.RegistryURL, imgPath)
}

// RegistryImageForVMI returns image name for VMI.
func (s *Settings) RegistryImageForVMI(name, namespace string) string {
	imgPath := path.Clean(fmt.Sprintf(VMIImageTmpl, namespace, name))
	return path.Join(s.RegistryURL, imgPath)
}

// RegistryImageForVMD returns image name for VMD.
func (s *Settings) RegistryImageForVMD(name, namespace string) string {
	imgPath := path.Clean(fmt.Sprintf(VMDImageTmpl, namespace, name))
	return path.Join(s.RegistryURL, imgPath)
}
