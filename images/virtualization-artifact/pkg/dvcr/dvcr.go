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

	"sigs.k8s.io/controller-runtime/pkg/client"
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
	// ImageMonitorSchedule is a cron schedule for periodic DVCR image presence checks.
	ImageMonitorSchedule string
	// GCSchedule is a cron formatted schedule to periodically run a garbage collection.
	GCSchedule string
}

type UploaderIngressSettings struct {
	Host               string
	TLSSecret          string
	TLSSecretNamespace string
	Class              string
}

const (
	CVMIImageTmpl     = "cvi/%s:%s"
	VMIImageTmpl      = "vi/%s/%s:%s"
	VMDImageTmpl      = "vd/%s/%s:%s"
	DefaultGCSchedule = "0 2 * * *" // Run DVCR garbage collect on 2:00 am every day.
)

// RegistryImageForCVI returns image name for CVI.
func (s *Settings) RegistryImageForCVI(obj client.Object) string {
	imgPath := path.Clean(fmt.Sprintf(CVMIImageTmpl, obj.GetName(), obj.GetUID()))
	return path.Join(s.RegistryURL, imgPath)
}

// RegistryImageForVI returns image name for VI.
func (s *Settings) RegistryImageForVI(obj client.Object) string {
	imgPath := path.Clean(fmt.Sprintf(VMIImageTmpl, obj.GetNamespace(), obj.GetName(), obj.GetUID()))
	return path.Join(s.RegistryURL, imgPath)
}

// RegistryImageForVD returns image name for VD.
func (s *Settings) RegistryImageForVD(obj client.Object) string {
	imgPath := path.Clean(fmt.Sprintf(VMDImageTmpl, obj.GetNamespace(), obj.GetName(), obj.GetUID()))
	return path.Join(s.RegistryURL, imgPath)
}
