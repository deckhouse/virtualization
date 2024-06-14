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

package config

import (
	"fmt"
	"os"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
)

func LoadDVCRSettingsFromEnvs(controllerNamespace string) (*dvcr.Settings, error) {
	dvcrSettings := &dvcr.Settings{
		AuthSecret:           os.Getenv(common.DVCRAuthSecretVar),
		AuthSecretNamespace:  os.Getenv(common.DVCRAuthSecretNSVar),
		CertsSecret:          os.Getenv(common.DVCRCertsSecretVar),
		CertsSecretNamespace: os.Getenv(common.DVCRCertsSecretNSVar),
		RegistryURL:          os.Getenv(common.DVCRRegistryURLVar),
		InsecureTLS:          os.Getenv(common.DVCRInsecureTLSVar),
		UploaderIngressSettings: dvcr.UploaderIngressSettings{
			Host:               os.Getenv(common.UploaderIngressHostVar),
			TLSSecret:          os.Getenv(common.UploaderIngressTLSSecretVar),
			TLSSecretNamespace: os.Getenv(common.UploaderIngressTLSSecretNS),
			Class:              os.Getenv(common.UploaderIngressClassVar),
		},
	}

	if dvcrSettings.RegistryURL == "" {
		return nil, fmt.Errorf("environment variable %q undefined, specify DVCR settings", common.DVCRRegistryURLVar)
	}
	if dvcrSettings.UploaderIngressSettings.Host == "" {
		return nil, fmt.Errorf("environment variable %q undefined, specify DVCR settings", common.UploaderIngressHostVar)
	}
	if dvcrSettings.UploaderIngressSettings.TLSSecret == "" {
		return nil, fmt.Errorf("environment variable %q undefined, specify DVCR settings", common.UploaderIngressTLSSecretVar)
	}
	if dvcrSettings.UploaderIngressSettings.Class == "" {
		return nil, fmt.Errorf("environment variable %q undefined, specify DVCR settings", common.UploaderIngressClassVar)
	}

	if dvcrSettings.AuthSecret != "" && dvcrSettings.AuthSecretNamespace == "" {
		dvcrSettings.AuthSecretNamespace = controllerNamespace
	}

	if dvcrSettings.CertsSecret != "" && dvcrSettings.CertsSecretNamespace == "" {
		dvcrSettings.CertsSecretNamespace = controllerNamespace
	}

	if dvcrSettings.UploaderIngressSettings.TLSSecret != "" && dvcrSettings.UploaderIngressSettings.TLSSecretNamespace == "" {
		dvcrSettings.UploaderIngressSettings.TLSSecretNamespace = controllerNamespace
	}

	return dvcrSettings, nil
}
