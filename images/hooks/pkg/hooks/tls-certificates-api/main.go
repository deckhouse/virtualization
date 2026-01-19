/*
Copyright 2025 Flant JSC

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

package tls_certificates_api

import (
	"fmt"
	"hooks/pkg/settings"

	tlscertificate "github.com/deckhouse/module-sdk/common-hooks/tls-certificate"
)

var _ = tlscertificate.RegisterInternalTLSHookEM(tlscertificate.GenSelfSignedTLSHookConf{
	CN:            settings.APICertCN,
	TLSSecretName: "virtualization-api-tls",
	Namespace:     settings.ModuleNamespace,
	SANs: tlscertificate.DefaultSANs([]string{
		"localhost",
		"127.0.0.1",
		// virtualization-api
		settings.APICertCN,
		// virtualization-api.d8-virtualization
		fmt.Sprintf("%s.%s", settings.APICertCN, settings.ModuleNamespace),
		// virtualization-api.d8-virtualization.svc
		fmt.Sprintf("%s.%s.svc", settings.APICertCN, settings.ModuleNamespace),
	}),

	FullValuesPathPrefix: fmt.Sprintf("%s.internal.apiserver.cert", settings.ModuleName),
	CommonCAValuesPath:   fmt.Sprintf("%s.internal.rootCA", settings.ModuleName),
})
