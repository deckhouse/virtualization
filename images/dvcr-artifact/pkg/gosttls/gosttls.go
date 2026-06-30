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

// Package gosttls optionally narrows the process-wide TLS 1.3 cipher suites to
// the hardware-accelerated AES-GCM / ChaCha20 set, removing the GOST
// (Kuznyechik/MGM) suites that the deckhouse GOST Go toolchain installs by
// default.
//
// The deckhouse toolchain advertises software GOST TLS 1.3 suites in every Go
// binary; when DVCR (which prefers GOST) is the peer, the importer/uploader
// upload runs through a pure-software GOST cipher and is capped at a few MB/s on
// a single core. Blank-import this package from a binary that should prefer
// AES, and build it with `-tags=dvcr_no_gost_tls` to activate the override.
//
// Without the build tag this package is a no-op, so standard-Go builds (local
// dev, golangci-lint) compile unchanged.
package gosttls
