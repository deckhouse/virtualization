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

package tls_certificates_api_proxy

import (
	"fmt"
	"hooks/pkg/settings"

	v1 "k8s.io/api/certificates/v1"

	tlscertificate "github.com/deckhouse/module-sdk/common-hooks/tls-certificate"
	"github.com/deckhouse/module-sdk/pkg"
)

var _ = tlscertificate.RegisterInternalTLSHookEM(tlscertificate.GenSelfSignedTLSHookConf{
	CN:                   settings.APIProxyCertCN,
	TLSSecretName:        "virtualization-api-proxy-tls",
	Namespace:            settings.ModuleNamespace,
	SANs:                 func(input *pkg.HookInput) []string { return []string{} },
	FullValuesPathPrefix: fmt.Sprintf("%s.internal.apiserver.proxyCert", settings.ModuleName),
	CommonCAValuesPath:   fmt.Sprintf("%s.internal.rootCA", settings.ModuleName),
	Usages:               []v1.KeyUsage{v1.UsageClientAuth},
})
