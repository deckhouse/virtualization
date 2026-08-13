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

package registry

import (
	"encoding/hex"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	importerrs "github.com/deckhouse/virtualization-controller/dvcr-importers/pkg/errors"
)

func Test_ParseChecksums(t *testing.T) {
	checksums, err := ParseChecksums("")
	require.NoError(t, err)
	require.Nil(t, checksums, "an empty value means no checksums to verify")

	checksums, err = ParseChecksums("md5:5D41402ABC4B2A76B9719D911017C592,streebog256:3fb0700a41ce6e41413ba764f98bf2135ba6ded516bea2fae8429cc5bdd46d6d")
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		"md5":         "5d41402abc4b2a76b9719d911017c592",
		"streebog256": "3fb0700a41ce6e41413ba764f98bf2135ba6ded516bea2fae8429cc5bdd46d6d",
	}, checksums)

	_, err = ParseChecksums("sha256")
	require.Error(t, err, "a pair without the algorithm separator is malformed")

	_, err = ParseChecksums("gost341194:abc")
	require.Error(t, err, "an algorithm the importer cannot calculate is rejected")
}

// Test_ChecksumAlgorithms verifies every supported algorithm against the sum of
// the "hello" string, so that a hash function is never wired to the wrong name.
func Test_ChecksumAlgorithms(t *testing.T) {
	sums := map[string]string{
		"md5":         "5d41402abc4b2a76b9719d911017c592",
		"sha1":        "aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d",
		"sha256":      "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
		"sha512":      "9b71d224bd62f3785d96d46ad3ea3d73319bfbc2890caadae2dff72519673ca72323c3d99ba5c11d7c7acc6e14b8c5da0c4663475c2e5c3adef46f73bcdec043",
		"streebog256": "3fb0700a41ce6e41413ba764f98bf2135ba6ded516bea2fae8429cc5bdd46d6d",
		"streebog512": "8df414260966beb7b34d920763079e15df1f63297eb3dd4311e8b585d4bf2f5923214f1dfed3fdee4aaf018330a12acde0efcc338eb52922f3e571212d42c8de",
	}

	require.Len(t, sums, len(checksumAlgorithms), "every supported algorithm needs a known sum to check against")

	for algorithm, expectedSum := range sums {
		newHash, ok := checksumAlgorithms[algorithm]
		require.True(t, ok, "algorithm %q is not supported", algorithm)

		hash := newHash()
		_, err := hash.Write([]byte("hello"))
		require.NoError(t, err)
		require.Equal(t, expectedSum, hex.EncodeToString(hash.Sum(nil)), "wrong sum for %q", algorithm)
	}
}

// Test_ChecksumVerifiers covers the verdict itself: the data is fed to the
// hashes exactly as the importer feeds it, and every algorithm has to agree
// before the image is accepted.
func Test_ChecksumVerifiers(t *testing.T) {
	const data = "hello"
	const (
		md5Sum         = "5d41402abc4b2a76b9719d911017c592"
		sha256Sum      = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
		streebog256Sum = "3fb0700a41ce6e41413ba764f98bf2135ba6ded516bea2fae8429cc5bdd46d6d"
	)

	run := func(t *testing.T, checksums map[string]string) []error {
		t.Helper()

		writers, checks := newChecksumVerifiers(checksums)
		require.Len(t, writers, len(checksums))
		require.Len(t, checks, len(checksums))

		if len(writers) > 0 {
			_, err := io.MultiWriter(writers...).Write([]byte(data))
			require.NoError(t, err)
		}

		errs := make([]error, 0, len(checks))
		for _, check := range checks {
			if err := check(); err != nil {
				errs = append(errs, err)
			}
		}

		return errs
	}

	t.Run("no checksums, nothing to verify", func(t *testing.T) {
		require.Empty(t, run(t, nil))
	})

	t.Run("every checksum matches", func(t *testing.T) {
		require.Empty(t, run(t, map[string]string{
			"md5":         md5Sum,
			"sha256":      sha256Sum,
			"streebog256": streebog256Sum,
		}))
	})

	t.Run("a mismatch names its own algorithm", func(t *testing.T) {
		errs := run(t, map[string]string{"streebog256": "0" + streebog256Sum[1:]})
		require.Len(t, errs, 1)

		var badChecksum importerrs.BadImageChecksumError
		require.ErrorAs(t, errs[0], &badChecksum)
		require.Equal(t, "BadImageChecksum", badChecksum.Reason())
		require.True(t, badChecksum.Permanent(), "a mismatch must not be retried")
		require.Equal(t,
			"streebog256 sum mismatch: 0fb0700a41ce6e41413ba764f98bf2135ba6ded516bea2fae8429cc5bdd46d6d != "+streebog256Sum,
			errs[0].Error(),
		)
	})

	t.Run("only the wrong algorithm complains", func(t *testing.T) {
		errs := run(t, map[string]string{
			"sha256": sha256Sum,
			"md5":    "0" + md5Sum[1:],
		})
		require.Len(t, errs, 1)
		require.Contains(t, errs[0].Error(), "md5 sum mismatch")
	})
}
