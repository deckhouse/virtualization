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

package importer

import (
	"github.com/deckhouse/virtualization-controller/pkg/common"
	dsutil "github.com/deckhouse/virtualization-controller/pkg/common/datasource"
	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	virtv2alpha1 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// Settings stores all possible settings for dvcr-importer binary.
// Fields from this struct are passed via environment variables.
type Settings struct {
	Verbose                string
	Endpoint               string
	MD5                    string
	SHA256                 string
	SecretName             string
	Source                 string
	ContentType            string
	ImageSize              string
	CertConfigMap          string
	DiskID                 string
	UUID                   string
	ReadyFile              string
	DoneFile               string
	BackingFile            string
	Thumbprint             string
	FilesystemOverhead     string
	AuthSecret             string
	InsecureTLS            bool
	HTTPProxy              string
	HTTPSProxy             string
	NoProxy                string
	CertConfigMapProxy     string
	ExtraHeaders           []string
	SecretExtraHeaders     []string
	DestinationEndpoint    string
	DestinationInsecureTLS string
	DestinationAuthSecret  string
}

func ApplyDVCRDestinationSettings(podEnvVars *Settings, dvcrSettings *dvcr.Settings, supGen *supplements.Generator, dvcrImageName string) {
	authSecret := dvcrSettings.AuthSecret
	if supplements.ShouldCopyDVCRAuthSecret(dvcrSettings, supGen) {
		authSecret = supGen.DVCRAuthSecret().Name
	}
	podEnvVars.DestinationAuthSecret = authSecret
	podEnvVars.DestinationInsecureTLS = dvcrSettings.InsecureTLS
	podEnvVars.DestinationEndpoint = dvcrImageName
}

// ApplyHTTPSourceSettings updates importer Pod settings to use http source.
func ApplyHTTPSourceSettings(podEnvVars *Settings, http *virtv2alpha1.DataSourceHTTP, supGen *supplements.Generator) {
	podEnvVars.Source = cc.SourceHTTP
	podEnvVars.Endpoint = http.URL

	if http.Checksum != nil {
		if http.Checksum.MD5 != "" {
			podEnvVars.MD5 = http.Checksum.MD5
		}

		if http.Checksum.SHA256 != "" {
			podEnvVars.SHA256 = http.Checksum.SHA256
		}
	}

	// Set ConfigMap name if caBundle is specified. ConfigMap will be created later after Pod start.
	if len(http.CABundle) > 0 {
		caBundleCM := supGen.CABundleConfigMap()
		podEnvVars.CertConfigMap = caBundleCM.Name
	}
}

// ApplyRegistrySourceSettings updates importer Pod settings to use registry source.
func ApplyRegistrySourceSettings(podEnvVars *Settings, ctrImg *virtv2alpha1.DataSourceContainerRegistry, supGen *supplements.Generator) {
	podEnvVars.Source = cc.SourceRegistry
	podEnvVars.Endpoint = common.DockerRegistrySchemePrefix + ctrImg.Image

	// Optional auth secret from imagePullSecret.
	if secretName := ctrImg.ImagePullSecret.Name; secretName != "" {
		// Copy imagePullSecret if resides in a different namespace.
		if dsutil.ShouldCopyImagePullSecret(ctrImg, supGen.Namespace) {
			imgPull := supGen.ImagePullSecret()
			podEnvVars.AuthSecret = imgPull.Name
		} else {
			podEnvVars.AuthSecret = secretName
		}
	}

	// Set ConfigMap name if caBundle is specified. ConfigMap will be created later after Pod start.
	if len(ctrImg.CABundle) > 0 {
		caBundleCM := supGen.CABundleConfigMap()
		podEnvVars.CertConfigMap = caBundleCM.Name
	}
}

// ApplyDVCRSourceSettings updates importer Pod settings to use dvcr registry source.
// NOTE: no auth secret required, it will be taken from DVCR destination settings.
func ApplyDVCRSourceSettings(podEnvVars *Settings, dvcrImageName string) {
	podEnvVars.Source = cc.SourceDVCR
	podEnvVars.Endpoint = dvcrImageName
}

// ApplyPVCSourceSettings updates importer Pod settings to use PVC source.
func ApplyPVCSourceSettings(podEnvVars *Settings) {
	podEnvVars.Source = "pvc"
}
