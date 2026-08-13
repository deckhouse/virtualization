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

package uploader

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Test_Options_Checksums covers the way the checksums reach the server: the
// controller passes them as a single environment variable, and a malformed value
// has to stop the uploader instead of silently verifying nothing.
func Test_Options_Checksums(t *testing.T) {
	t.Run("checksums are parsed", func(t *testing.T) {
		o := &Options{
			DestinationEndpoint: "registry.example.com/vi/default/image:latest",
			Checksums:           "sha256:2CF24DBA5FB0A30E26E83B2AC5B9E29E1B161E5C1FA7425E73043362938B9824,md5:5d41402abc4b2a76b9719d911017c592",
		}

		server, err := o.Complete()
		require.NoError(t, err)
		require.Equal(t, map[string]string{
			"md5":    "5d41402abc4b2a76b9719d911017c592",
			"sha256": "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
		}, server.checksums)
	})

	t.Run("no checksums means no verification", func(t *testing.T) {
		o := &Options{DestinationEndpoint: "registry.example.com/vi/default/image:latest"}

		server, err := o.Complete()
		require.NoError(t, err)
		require.Empty(t, server.checksums)
	})

	t.Run("an unsupported algorithm is rejected", func(t *testing.T) {
		o := &Options{
			DestinationEndpoint: "registry.example.com/vi/default/image:latest",
			Checksums:           "sha3:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
		}

		_, err := o.Complete()
		require.ErrorContains(t, err, "unsupported checksum algorithm")
	})

	t.Run("a malformed value is rejected", func(t *testing.T) {
		o := &Options{
			DestinationEndpoint: "registry.example.com/vi/default/image:latest",
			Checksums:           "sha256",
		}

		_, err := o.Complete()
		require.ErrorContains(t, err, "malformed checksum")
	})
}
