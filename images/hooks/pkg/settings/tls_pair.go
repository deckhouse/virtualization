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

package settings

import (
	"context"
	"crypto/x509"
	"fmt"

	tlscertificate "github.com/deckhouse/module-sdk/common-hooks/tls-certificate"
	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/certificate"
	objectpatch "github.com/deckhouse/module-sdk/pkg/object-patch"
	"github.com/deckhouse/module-sdk/pkg/registry"
)

// RegisterTLSHook registers a tls-certificates-* hook in the module queue, the same one
// ca-discovery uses, so every writer of the common CA runs serialized.
func RegisterTLSHook(conf tlscertificate.GenSelfSignedTLSHookConf) bool {
	return RegisterTLSHookFunc(conf, tlscertificate.GenSelfSignedTLS(conf))
}

// RegisterTLSHookFunc is RegisterTLSHook with a custom handler wrapping the sdk one.
func RegisterTLSHookFunc(conf tlscertificate.GenSelfSignedTLSHookConf, fn pkg.HookFunc[*pkg.HookInput]) bool {
	config := tlscertificate.GenSelfSignedTLSConfig(conf)
	config.Queue = ModuleQueue

	return registry.RegisterFunc(config, fn)
}

// TLSBeforeHookCheck builds the BeforeHookCheck shared by the tls-certificates-* hooks:
// skip until the ModuleConfig is available and reissue the secret when its pair is broken.
func TLSBeforeHookCheck(component, secretName string) func(input *pkg.HookInput) bool {
	return func(input *pkg.HookInput) bool {
		canRun, err := CanRunWithModuleConfig(context.Background(), input)
		if err != nil {
			input.Logger.Error(fmt.Sprintf("Check module config before %s TLS hook", component), "error", err)
			return false
		}
		if !canRun {
			return false
		}

		ReissueBrokenTLSPair(input, secretName)

		return true
	}
}

// ReissueBrokenTLSPair deletes the TLS secret from the hook snapshot when its leaf is not
// signed by its ca.crt or does not match its key. The shared sdk hook only compares the
// CA bytes with the common CA, so such a pair would otherwise be kept until it expires.
// The deletion triggers the hook again with an empty snapshot, which reissues the pair.
func ReissueBrokenTLSPair(input *pkg.HookInput, secretName string) {
	certs, err := objectpatch.UnmarshalToStruct[certificate.Certificate](input.Snapshots, tlscertificate.InternalTLSSnapshotKey)
	if err != nil || len(certs) == 0 {
		return
	}

	if err := VerifyTLSPair(certs[0]); err != nil {
		input.Logger.Warn("TLS secret holds a pair that does not verify against its own CA, deleting it to reissue",
			"secret", secretName, "error", err)
		input.PatchCollector.Delete("v1", "Secret", ModuleNamespace, secretName)
	}
}

// VerifyTLSPair checks that the key matches the leaf and that the leaf is signed by the CA.
func VerifyTLSPair(pair certificate.Certificate) error {
	ca, leaf, err := certificate.ParseCertificatesFromPEM(pair.CA, pair.Cert, pair.Key)
	if err != nil {
		return err
	}

	leafX509, err := x509.ParseCertificate(leaf.Certificate[0])
	if err != nil {
		return fmt.Errorf("parse leaf certificate: %w", err)
	}

	return leafX509.CheckSignatureFrom(ca)
}
