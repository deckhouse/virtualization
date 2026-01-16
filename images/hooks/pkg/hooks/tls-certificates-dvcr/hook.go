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

package tls_certificates_dvcr

import (
	"fmt"

	"hooks/pkg/settings"

	"github.com/tidwall/gjson"

	tlscertificate "github.com/deckhouse/module-sdk/common-hooks/tls-certificate"
	"github.com/deckhouse/module-sdk/pkg"
)

func dvcrGetServiceIP(input *pkg.HookInput) gjson.Result {
	return input.Values.Get(fmt.Sprintf("%s.internal.dvcr.serviceIP", settings.ModuleName))
}

func dvcrSANs(sans []string) tlscertificate.SANsGenerator {
	return func(input *pkg.HookInput) []string {
		return append(sans, []string{"localhost", "127.0.0.1", dvcrGetServiceIP(input).String()}...)
	}
}

var _ = tlscertificate.RegisterInternalTLSHookEM(tlscertificate.GenSelfSignedTLSHookConf{
	CN:            settings.DVCRCertCN,
	TLSSecretName: "dvcr-tls",
	Namespace:     settings.ModuleNamespace,

	SANs: dvcrSANs([]string{
		settings.DVCRCertCN,
		fmt.Sprintf("%s.%s", settings.DVCRCertCN, settings.ModuleNamespace),
		fmt.Sprintf("%s.%s.svc", settings.DVCRCertCN, settings.ModuleNamespace),
	}),

	FullValuesPathPrefix: fmt.Sprintf("%s.internal.dvcr.cert", settings.ModuleName),
	CommonCAValuesPath:   fmt.Sprintf("%s.internal.rootCA", settings.ModuleName),

	BeforeHookCheck: func(input *pkg.HookInput) bool {
		return dvcrGetServiceIP(input).Type != gjson.Null
	},
})
