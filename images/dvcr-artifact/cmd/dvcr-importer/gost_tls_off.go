//go:build dvcr_no_gost_tls

/*
Copyright 2026 Flant JSC

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

// This file is compiled only when built with the deckhouse GOST-enabled Go
// toolchain (golang-alt/golang-debian) via `-tags=dvcr_no_gost_tls`. That
// toolchain unconditionally installs GOST (Kuznyechik/MGM) TLS 1.3 cipher
// suites, which are pure-software (no hardware acceleration) and cap the
// importer's upload to the in-cluster DVCR at a few MB/s on a single core.
//
// SetAllowedTLS13CipherSuites is a toolchain-specific API (absent from upstream
// Go), so the call is isolated behind a build tag: standard-Go builds (local
// dev, golangci-lint) simply exclude this file and keep upstream behaviour.

package main

import "crypto/tls"

func init() {
	// Restrict TLS 1.3 to the AES-NI / ChaCha20 accelerated suites so GOST is
	// no longer advertised and a GOST-preferring DVCR cannot select it.
	tls.SetAllowedTLS13CipherSuites([]uint16{
		tls.TLS_AES_128_GCM_SHA256,
		tls.TLS_AES_256_GCM_SHA384,
		tls.TLS_CHACHA20_POLY1305_SHA256,
	})
}
