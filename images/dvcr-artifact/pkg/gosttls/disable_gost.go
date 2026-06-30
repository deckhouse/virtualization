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

package gosttls

import "crypto/tls"

// SetAllowedTLS13CipherSuites is a deckhouse-toolchain-only API (absent from
// upstream Go), so this call lives behind the dvcr_no_gost_tls build tag:
// builds without the tag exclude this file and keep upstream behaviour.
func init() {
	tls.SetAllowedTLS13CipherSuites([]uint16{
		tls.TLS_AES_128_GCM_SHA256,
		tls.TLS_AES_256_GCM_SHA384,
		tls.TLS_CHACHA20_POLY1305_SHA256,
	})
}
