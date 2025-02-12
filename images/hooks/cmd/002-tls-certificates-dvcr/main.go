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

package main

import (
	"fmt"
	"hooks/pkg/common"

	tlscertificate "github.com/deckhouse/module-sdk/common-hooks/tls-certificate"
	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/app"
	"github.com/tidwall/gjson"
)

func dvcrGetServiceIP(input *pkg.HookInput) gjson.Result {
	return input.Values.Get(fmt.Sprintf("%s.internal.dvcr.serviceIP", common.MODULE_NAME))
}

func dvcrSANs(sans []string) tlscertificate.SANsGenerator {
	return func(input *pkg.HookInput) []string {
		return append(sans, []string{"localhost", "127.0.0.1", dvcrGetServiceIP(input).String()}...)
	}
}

var _ = tlscertificate.RegisterInternalTLSHookEM(tlscertificate.GenSelfSignedTLSHookConf{
	CN:            common.DVCR_CERT_CN,
	TLSSecretName: "dvcr-tls",
	Namespace:     common.MODULE_NAMESPACE,

	SANs: dvcrSANs([]string{
		common.DVCR_CERT_CN,
		fmt.Sprintf("%s.%s", common.DVCR_CERT_CN, common.MODULE_NAMESPACE),
		fmt.Sprintf("%s.%s.svc", common.DVCR_CERT_CN, common.MODULE_NAMESPACE),
	}),

	FullValuesPathPrefix: fmt.Sprintf("%s.internal.dvcr.cert", common.MODULE_NAME),
	CommonCAValuesPath:   fmt.Sprintf("%s.internal.rootCA", common.MODULE_NAME),

	BeforeHookCheck: func(input *pkg.HookInput) bool {
		if dvcrGetServiceIP(input).Type == gjson.Null {
			return false
		}

		return true
	},
})

func main() {
	app.Run()
}
