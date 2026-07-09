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
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/dvcr/registrytoken"
)

type Settings struct {
	// ControllerNamespace is the namespace where the virtualization-controller runs.
	ControllerNamespace string
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
	// TokenSigner mints the scoped per-namespace DVCR tokens: importer and uploader
	// Pods authenticate with a token minted for the single repository they use,
	// instead of the shared read-write credential, which is then no longer copied
	// into tenant namespaces.
	TokenSigner *registrytoken.Signer
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

// RepoPath extracts the repository path (e.g. "vi/ns/name", "cvi/name") from a
// DVCR image reference by stripping the optional docker:// scheme, the registry
// host and the tag/digest. It is the name used in a scoped token's access claim.
func (s *Settings) RepoPath(imageRef string) string {
	ref := strings.TrimPrefix(imageRef, "docker://")
	ref = strings.TrimPrefix(ref, s.RegistryURL)
	ref = strings.TrimPrefix(ref, "/")
	if i := strings.Index(ref, "@"); i >= 0 {
		ref = ref[:i]
	}
	if slash := strings.LastIndex(ref, "/"); strings.LastIndex(ref, ":") > slash {
		ref = ref[:strings.LastIndex(ref, ":")]
	}
	return path.Clean(ref)
}
