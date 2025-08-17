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

package uploader

import (
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
)

// Settings stores all possible settings for dvcr-uploader binary.
// Fields from this struct are passed via environment variables.
type Settings struct {
	Verbose                string
	SecretExtraHeaders     []string
	DestinationEndpoint    string
	DestinationInsecureTLS string
	DestinationAuthSecret  string
}

func ApplyDVCRDestinationSettings(podEnvVars *Settings, dvcrSettings *dvcr.Settings, supGen supplements.Generator, dvcrImageName string) {
	authSecret := dvcrSettings.AuthSecret
	if supplements.ShouldCopyDVCRAuthSecret(dvcrSettings, supGen) {
		authSecret = supGen.DVCRAuthSecret().Name
	}
	podEnvVars.DestinationAuthSecret = authSecret
	podEnvVars.DestinationInsecureTLS = dvcrSettings.InsecureTLS
	podEnvVars.DestinationEndpoint = dvcrImageName
}
