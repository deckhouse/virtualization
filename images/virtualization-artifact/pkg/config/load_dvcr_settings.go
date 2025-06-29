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

	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
)

const (
	// DVCRRegistryURLVar is an env variable holds address to DVCR registry.
	DVCRRegistryURLVar = "DVCR_REGISTRY_URL"
	// DVCRAuthSecretVar is an env variable holds the name of the Secret with DVCR auth credentials.
	DVCRAuthSecretVar = "DVCR_AUTH_SECRET"
	// DVCRAuthSecretNSVar is an env variable holds the namespace for the Secret with DVCR auth credentials.
	DVCRAuthSecretNSVar = "DVCR_AUTH_SECRET_NAMESPACE"
	// DVCRCertsSecretVar is an env variable holds the name of the Secret with DVCR certificates.
	DVCRCertsSecretVar = "DVCR_CERTS_SECRET"
	// DVCRCertsSecretNSVar is an env variable holds the namespace for the Secret with DVCR certificates.
	DVCRCertsSecretNSVar = "DVCR_CERTS_SECRET_NAMESPACE"
	// DVCRInsecureTLSVar is an env variable holds the flag whether DVCR is insecure.
	DVCRInsecureTLSVar = "DVCR_INSECURE_TLS"

	// IngressHostVar is a env variable
	IngressHostVar = "INGRESS_HOST"
	// IngressTLSSecretVar is a env variable
	IngressTLSSecretVar = "INGRESS_TLS_SECRET"
	// IngressClassVar is a env variable
	IngressClassVar = "INGRESS_CLASS"
	// IngressTLSSecretNS is a env variable
	IngressTLSSecretNS = "INGRESS_TLS_SECRET_NAMESPACE"
)

func LoadDVCRSettingsFromEnvs(controllerNamespace string) (*dvcr.Settings, error) {
	dvcrSettings := &dvcr.Settings{
		AuthSecret:           os.Getenv(DVCRAuthSecretVar),
		AuthSecretNamespace:  os.Getenv(DVCRAuthSecretNSVar),
		CertsSecret:          os.Getenv(DVCRCertsSecretVar),
		CertsSecretNamespace: os.Getenv(DVCRCertsSecretNSVar),
		RegistryURL:          os.Getenv(DVCRRegistryURLVar),
		InsecureTLS:          os.Getenv(DVCRInsecureTLSVar),
		IngressSettings: dvcr.IngressSettings{
			Host:               os.Getenv(IngressHostVar),
			TLSSecret:          os.Getenv(IngressTLSSecretVar),
			TLSSecretNamespace: os.Getenv(IngressTLSSecretNS),
			Class:              os.Getenv(IngressClassVar),
		},
	}

	if dvcrSettings.RegistryURL == "" {
		return nil, fmt.Errorf("environment variable %q undefined, specify DVCR settings", DVCRRegistryURLVar)
	}
	if dvcrSettings.IngressSettings.Host == "" {
		return nil, fmt.Errorf("environment variable %q undefined, specify DVCR settings", IngressHostVar)
	}
	if dvcrSettings.IngressSettings.TLSSecret == "" {
		return nil, fmt.Errorf("environment variable %q undefined, specify DVCR settings", IngressTLSSecretVar)
	}
	if dvcrSettings.IngressSettings.Class == "" {
		return nil, fmt.Errorf("environment variable %q undefined, specify DVCR settings", IngressClassVar)
	}

	if dvcrSettings.AuthSecret != "" && dvcrSettings.AuthSecretNamespace == "" {
		dvcrSettings.AuthSecretNamespace = controllerNamespace
	}

	if dvcrSettings.CertsSecret != "" && dvcrSettings.CertsSecretNamespace == "" {
		dvcrSettings.CertsSecretNamespace = controllerNamespace
	}

	if dvcrSettings.IngressSettings.TLSSecret != "" && dvcrSettings.IngressSettings.TLSSecretNamespace == "" {
		dvcrSettings.IngressSettings.TLSSecretNamespace = controllerNamespace
	}

	return dvcrSettings, nil
}
