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
	CVMIImageTmpl = "cvi/%s"
	VMIImageTmpl  = "vi/%s/%s"
	VMDImageTmpl  = "vd/%s/%s"
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
