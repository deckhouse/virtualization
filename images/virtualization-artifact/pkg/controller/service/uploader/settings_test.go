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

	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func Test_ApplyUploadSourceSettings(t *testing.T) {
	tests := []struct {
		name   string
		upload *v1alpha2.DataSourceUpload
		want   map[string]string
	}{
		{
			name:   "no upload section",
			upload: nil,
		},
		{
			name:   "upload section without checksums",
			upload: &v1alpha2.DataSourceUpload{},
		},
		{
			name: "upload section with checksums",
			upload: &v1alpha2.DataSourceUpload{Checksum: &v1alpha2.Checksum{
				SHA256:      "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
				Streebog256: "3f539a213e97c802cc229d474c6aa32a825a360b2a933a949fd925208d9ce1bb",
			}},
			want: map[string]string{
				"sha256":      "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
				"streebog256": "3f539a213e97c802cc229d474c6aa32a825a360b2a933a949fd925208d9ce1bb",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var settings Settings
			ApplyUploadSourceSettings(&settings, test.upload)

			require.Equal(t, test.want, settings.Checksums)
		})
	}
}

// Test_UploaderContainerEnv_Checksums makes sure the checksums reach the Pod:
// settings that never become an environment variable are settings the uploader
// cannot act on.
func Test_UploaderContainerEnv_Checksums(t *testing.T) {
	withChecksums := factory{podSettings: PodSettings{Checksums: map[string]string{
		"md5":  "5d41402abc4b2a76b9719d911017c592",
		"sha1": "aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d",
	}}}

	var value string
	for _, env := range withChecksums.uploaderContainerEnv() {
		if env.Name == common.UploaderChecksums {
			value = env.Value
		}
	}
	require.Equal(t, "md5:5d41402abc4b2a76b9719d911017c592,sha1:aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d", value)

	withoutChecksums := factory{}
	for _, env := range withoutChecksums.uploaderContainerEnv() {
		require.NotEqual(t, common.UploaderChecksums, env.Name,
			"an upload without checksums must not set the variable at all")
	}
}
