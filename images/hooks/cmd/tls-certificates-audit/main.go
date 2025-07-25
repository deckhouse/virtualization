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
	"context"
	"fmt"

	"hooks/pkg/common"

	tlscertificate "github.com/deckhouse/module-sdk/common-hooks/tls-certificate"
	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/app"
	"github.com/deckhouse/module-sdk/pkg/registry"
)

var conf = tlscertificate.GenSelfSignedTLSHookConf{
	CN:            common.AUDIT_CERT_CN,
	TLSSecretName: "virtualization-audit-tls",
	Namespace:     common.MODULE_NAMESPACE,
	SANs: tlscertificate.DefaultSANs([]string{
		"localhost",
		"127.0.0.1",
		// virtualization-audit
		common.AUDIT_CERT_CN,
		// virtualization-audit.d8-virtualization
		fmt.Sprintf("%s.%s", common.AUDIT_CERT_CN, common.MODULE_NAMESPACE),
		// virtualization-audit.d8-virtualization.svc
		fmt.Sprintf("%s.%s.svc", common.AUDIT_CERT_CN, common.MODULE_NAMESPACE),
		// virtualization-audit.d8-virtualization.svc.cluster.local
		tlscertificate.ClusterDomainSAN(fmt.Sprintf("%s.%s.svc", common.AUDIT_CERT_CN, common.MODULE_NAMESPACE)),
	}),

	FullValuesPathPrefix: fmt.Sprintf("%s.internal.audit.cert", common.MODULE_NAME),
	CommonCAValuesPath:   fmt.Sprintf("%s.internal.rootCA", common.MODULE_NAME),
}

var genSelfSignedTLS = func(conf tlscertificate.GenSelfSignedTLSHookConf) pkg.ReconcileFunc {
	return func(ctx context.Context, input *pkg.HookInput) error {
		if !input.Values.Get("virtualization.audit.enabled").Bool() {
			return nil
		}

		return tlscertificate.GenSelfSignedTLS(conf)(ctx, input)
	}
}

var _ = registry.RegisterFunc(tlscertificate.GenSelfSignedTLSConfig(conf), genSelfSignedTLS(conf))

func main() {
	app.Run()
}
