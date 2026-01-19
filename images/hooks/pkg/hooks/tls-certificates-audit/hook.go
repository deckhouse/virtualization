//go:build EE
// +build EE

/*
Copyright 2025 Flant JSC
Licensed under the Deckhouse Platform Enterprise Edition (EE) license. See https://github.com/deckhouse/deckhouse/blob/main/ee/LICENSE
*/

package tls_certificates_audit

import (
	"context"
	"fmt"

	tlscertificate "github.com/deckhouse/module-sdk/common-hooks/tls-certificate"
	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/registry"
	"hooks/pkg/settings"
)

var conf = tlscertificate.GenSelfSignedTLSHookConf{
	CN:            settings.AuditCertCN,
	TLSSecretName: "virtualization-audit-tls",
	Namespace:     settings.ModuleNamespace,
	SANs: tlscertificate.DefaultSANs([]string{
		"localhost",
		"127.0.0.1",
		// virtualization-audit
		settings.AuditCertCN,
		// virtualization-audit.d8-virtualization
		fmt.Sprintf("%s.%s", settings.AuditCertCN, settings.ModuleNamespace),
		// virtualization-audit.d8-virtualization.svc
		fmt.Sprintf("%s.%s.svc", settings.AuditCertCN, settings.ModuleNamespace),
		// virtualization-audit.d8-virtualization.svc.cluster.local
		tlscertificate.ClusterDomainSAN(fmt.Sprintf("%s.%s.svc", settings.AuditCertCN, settings.ModuleNamespace)),
	}),

	FullValuesPathPrefix: fmt.Sprintf("%s.internal.audit.cert", settings.ModuleName),
	CommonCAValuesPath:   fmt.Sprintf("%s.internal.rootCA", settings.ModuleName),
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
