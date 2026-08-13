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

package importer

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func Test_ApplyHTTPSourceSettings(t *testing.T) {
	supgen := supplements.NewGenerator("vi", "image", "default", "uid")

	t.Run("checksums reach the settings", func(t *testing.T) {
		var settings Settings
		ApplyHTTPSourceSettings(&settings, &v1alpha2.DataSourceHTTP{
			URL: "https://mirror.example.com/image.qcow2",
			Checksum: &v1alpha2.Checksum{
				SHA256:      "2CF24DBA5FB0A30E26E83B2AC5B9E29E1B161E5C1FA7425E73043362938B9824",
				Streebog512: "8df414260966beb7b34d920763079e15df1f63297eb3dd4311e8b585d4bf2f5923214f1dfed3fdee4aaf018330a12acde0efcc338eb52922f3e571212d42c8de",
			},
		}, supgen)

		require.Equal(t, SourceHTTP, settings.Source)
		require.Equal(t, "https://mirror.example.com/image.qcow2", settings.Endpoint)
		require.Equal(t, map[string]string{
			"sha256":      "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
			"streebog512": "8df414260966beb7b34d920763079e15df1f63297eb3dd4311e8b585d4bf2f5923214f1dfed3fdee4aaf018330a12acde0efcc338eb52922f3e571212d42c8de",
		}, settings.Checksums)
	})

	t.Run("a source without checksums asks for no verification", func(t *testing.T) {
		var settings Settings
		ApplyHTTPSourceSettings(&settings, &v1alpha2.DataSourceHTTP{
			URL: "https://mirror.example.com/image.qcow2",
		}, supgen)

		require.Nil(t, settings.Checksums)
	})
}

// Test_ImporterContainerEnv_Checksums makes sure the checksums reach the Pod:
// settings that never become an environment variable are settings the importer
// cannot act on.
func Test_ImporterContainerEnv_Checksums(t *testing.T) {
	withChecksums := Importer{
		PodSettings: &PodSettings{},
		EnvSettings: &Settings{Checksums: map[string]string{
			"md5":  "5d41402abc4b2a76b9719d911017c592",
			"sha1": "aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d",
		}},
	}

	var value string
	for _, env := range withChecksums.makeImporterContainerEnv() {
		if env.Name == common.ImporterChecksums {
			value = env.Value
		}
	}
	require.Equal(t, "md5:5d41402abc4b2a76b9719d911017c592,sha1:aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d", value)

	withoutChecksums := Importer{PodSettings: &PodSettings{}, EnvSettings: &Settings{}}
	for _, env := range withoutChecksums.makeImporterContainerEnv() {
		require.NotEqual(t, common.ImporterChecksums, env.Name,
			"an import without checksums must not set the variable at all")
	}
}
