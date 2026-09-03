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
	"github.com/deckhouse/virtualization-controller/pkg/dvcr/registrytoken"
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
	// DVCRImageMonitorScheduleVar is an env variable holds the cron schedule for DVCR image monitoring.
	DVCRImageMonitorScheduleVar = "DVCR_IMAGE_MONITOR_SCHEDULE"
	// DVCRGCScheduleVar is an env variable holds the cron schedule to run DVCR garbage collection.
	DVCRGCScheduleVar = "DVCR_GC_SCHEDULE"
	// DVCRTokenPrivateKeyVar holds the PEM ECDSA private key used to mint scoped
	// per-namespace DVCR tokens.
	DVCRTokenPrivateKeyVar = "DVCR_TOKEN_PRIVATE_KEY"

	// UploaderIngressHostVar is a env variable
	UploaderIngressHostVar = "UPLOADER_INGRESS_HOST"
	// UploaderIngressTLSSecretVar is a env variable
	UploaderIngressTLSSecretVar = "UPLOADER_INGRESS_TLS_SECRET"
	// UploaderIngressClassVar is a env variable
	UploaderIngressClassVar = "UPLOADER_INGRESS_CLASS"
	// UploaderIngressTLSSecretNSVar is a env variable
	UploaderIngressTLSSecretNSVar = "UPLOADER_INGRESS_TLS_SECRET_NAMESPACE"

	// UploaderListenerHostVar is an env variable holds the host image upload is published on through the Gateway API.
	UploaderListenerHostVar = "UPLOADER_LISTENER_HOST"
	// UploaderListenerSetNameVar is an env variable holds the name of the ListenerSet that publishes the upload host.
	UploaderListenerSetNameVar = "UPLOADER_LISTENER_SET_NAME"
	// UploaderListenerNameVar is an env variable holds the name of the listener the per-upload HTTPRoutes attach to.
	UploaderListenerNameVar = "UPLOADER_LISTENER_NAME"
	// UploaderListenerTLSSecretVar is an env variable holds the name of the Secret the listener terminates TLS with.
	UploaderListenerTLSSecretVar = "UPLOADER_LISTENER_TLS_SECRET"

	// UploaderIngressNamespaceVar is the namespace of the Deckhouse ingress-nginx module that
	// proxies user uploads to the uploader pod. Used to scope the uploader CiliumNetworkPolicy
	// ingress in network-isolated projects.
	UploaderIngressNamespaceVar = "UPLOADER_INGRESS_NAMESPACE"
	// UploaderGatewayNamespaceVar is the namespace of the Gateway API data-plane (the alb
	// module) that proxies user uploads when the UploadViaAPIGateway feature gate is on. Used
	// to scope the uploader CiliumNetworkPolicy ingress in network-isolated projects.
	UploaderGatewayNamespaceVar = "UPLOADER_GATEWAY_NAMESPACE"
)

func LoadDVCRSettingsFromEnvs(controllerNamespace string) (*dvcr.Settings, error) {
	dvcrSettings := &dvcr.Settings{
		ControllerNamespace:  controllerNamespace,
		AuthSecret:           os.Getenv(DVCRAuthSecretVar),
		AuthSecretNamespace:  os.Getenv(DVCRAuthSecretNSVar),
		CertsSecret:          os.Getenv(DVCRCertsSecretVar),
		CertsSecretNamespace: os.Getenv(DVCRCertsSecretNSVar),
		RegistryURL:          os.Getenv(DVCRRegistryURLVar),
		InsecureTLS:          os.Getenv(DVCRInsecureTLSVar),
		ImageMonitorSchedule: os.Getenv(DVCRImageMonitorScheduleVar),
		GCSchedule:           os.Getenv(DVCRGCScheduleVar),
		UploaderIngressSettings: dvcr.UploaderIngressSettings{
			Host:               os.Getenv(UploaderIngressHostVar),
			TLSSecret:          os.Getenv(UploaderIngressTLSSecretVar),
			TLSSecretNamespace: os.Getenv(UploaderIngressTLSSecretNSVar),
			Class:              os.Getenv(UploaderIngressClassVar),
		},
		UploaderListenerSetSettings: dvcr.UploaderListenerSetSettings{
			Host:          os.Getenv(UploaderListenerHostVar),
			Name:          os.Getenv(UploaderListenerSetNameVar),
			Namespace:     controllerNamespace,
			ListenerName:  os.Getenv(UploaderListenerNameVar),
			TLSSecretName: os.Getenv(UploaderListenerTLSSecretVar),
		},
		UploaderIngressNamespace: os.Getenv(UploaderIngressNamespaceVar),
		UploaderGatewayNamespace: os.Getenv(UploaderGatewayNamespaceVar),
	}

	if dvcrSettings.RegistryURL == "" {
		return nil, fmt.Errorf("environment variable %q undefined, specify DVCR settings", DVCRRegistryURLVar)
	}

	// Both upload hosts are optional: the module chart derives them from
	// global.modules.publicDomainTemplate and omits them when it is not set. An
	// uploader without a host is exposed by its Service only — no Ingress and no
	// HTTPRoute is created — and the upload goes through the in-cluster URL.
	if dvcrSettings.AuthSecret != "" && dvcrSettings.AuthSecretNamespace == "" {
		dvcrSettings.AuthSecretNamespace = controllerNamespace
	}

	if dvcrSettings.CertsSecret != "" && dvcrSettings.CertsSecretNamespace == "" {
		dvcrSettings.CertsSecretNamespace = controllerNamespace
	}

	if dvcrSettings.UploaderIngressSettings.TLSSecret != "" && dvcrSettings.UploaderIngressSettings.TLSSecretNamespace == "" {
		dvcrSettings.UploaderIngressSettings.TLSSecretNamespace = controllerNamespace
	}

	if dvcrSettings.GCSchedule == "" {
		dvcrSettings.GCSchedule = dvcr.DefaultGCSchedule
	}

	keyPEM := os.Getenv(DVCRTokenPrivateKeyVar)
	if keyPEM == "" {
		return nil, fmt.Errorf("environment variable %q undefined, specify DVCR settings", DVCRTokenPrivateKeyVar)
	}
	signer, err := registrytoken.NewSignerFromPEM([]byte(keyPEM))
	if err != nil {
		return nil, fmt.Errorf("init DVCR token signer: %w", err)
	}
	dvcrSettings.TokenSigner = signer

	return dvcrSettings, nil
}
