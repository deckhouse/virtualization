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

package datasource

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func Test_Checksums(t *testing.T) {
	require.Nil(t, Checksums(nil), "no checksum section means no checksums")
	require.Nil(t, Checksums(&v1alpha2.Checksum{}), "an empty checksum section means no checksums")

	checksums := Checksums(&v1alpha2.Checksum{
		SHA256:      "2CF24DBA5FB0A30E26E83B2AC5B9E29E1B161E5C1FA7425E73043362938B9824",
		Streebog512: "8df414260966beb7b34d920763079e15df1f63297eb3dd4311e8b585d4bf2f5923214f1dfed3fdee4aaf018330a12acde0efcc338eb52922f3e571212d42c8de",
	})

	require.Equal(t, map[string]string{
		"sha256":      "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
		"streebog512": "8df414260966beb7b34d920763079e15df1f63297eb3dd4311e8b585d4bf2f5923214f1dfed3fdee4aaf018330a12acde0efcc338eb52922f3e571212d42c8de",
	}, checksums, "checksums are collected from the spec and lowercased")
}

// Test_Checksums_EveryAlgorithm guards the mapping itself: a checksum a user
// puts into the spec but the map forgets is silently never verified.
func Test_Checksums_EveryAlgorithm(t *testing.T) {
	checksums := Checksums(&v1alpha2.Checksum{
		MD5:         "5d41402abc4b2a76b9719d911017c592",
		SHA1:        "aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d",
		SHA256:      "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
		SHA512:      "9b71d224bd62f3785d96d46ad3ea3d73319bfbc2890caadae2dff72519673ca72323c3d99ba5c11d7c7acc6e14b8c5da0c4663475c2e5c3adef46f73bcdec043",
		Streebog256: "3f539a213e97c802cc229d474c6aa32a825a360b2a933a949fd925208d9ce1bb",
		Streebog512: "8e945da209aa869f0455928529bcae4679e9873ab707b55315f56ceb98bef0a7362f715528356ee83cda5f2aac4c6ad2ba3a715c1bcd81cb8e9f90bf4c1c1a8a",
	})

	require.Len(t, checksums, 6, "every algorithm of the checksum section has to reach the Pod")
}

func Test_FormatChecksums(t *testing.T) {
	require.Empty(t, FormatChecksums(nil), "no checksums produce no environment variable value")

	// The order is alphabetical, otherwise the Pod specification would change
	// between reconciliations for no reason.
	require.Equal(t,
		"md5:5d41402abc4b2a76b9719d911017c592,sha1:aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d",
		FormatChecksums(map[string]string{
			"sha1": "aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d",
			"md5":  "5d41402abc4b2a76b9719d911017c592",
		}),
	)
}
