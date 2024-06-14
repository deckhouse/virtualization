/*
Copyright 2024 Flant JSC

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

package rewriter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidatingRename(t *testing.T) {
	tests := []struct {
		name     string
		manifest string
		expect   string
	}{
		{
			"mixed resources",
			`{"webhooks":[{"rules":[{"apiGroups":[""],"resources":["pods"]},{"apiGroups": ["original.group.io"], "resources": ["someresources"]}]}]}`,
			`{"webhooks":[{"rules":[{"apiGroups":[""],"resources":["pods"]},{"apiGroups": ["prefixed.resources.group.io"], "resources": ["prefixedsomeresources"]}]}]}`,
		},
		{
			"empty object",
			`{}`,
			`{}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rwr := createTestRewriter()

			resBytes, err := RewriteValidatingOrList(rwr.Rules, []byte(tt.manifest), Rename)
			require.NoError(t, err, "should rename validating webhook configuration")

			actual := string(resBytes)
			require.Equal(t, tt.expect, actual)
		})
	}
}

func TestValidatingRestore(t *testing.T) {
	tests := []struct {
		name     string
		manifest string
		expect   string
	}{
		{
			"mixed resources",
			`{"webhooks":[{"rules":[{"apiGroups":[""],"resources":["pods"]},{"apiGroups": ["prefixed.resources.group.io"], "resources": ["prefixedsomeresources"]}]}]}`,
			`{"webhooks":[{"rules":[{"apiGroups":[""],"resources":["pods"]},{"apiGroups": ["original.group.io"], "resources": ["someresources"]}]}]}`,
		},
		{
			"empty object",
			`{}`,
			`{}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rwr := createTestRewriter()

			resBytes, err := RewriteValidatingOrList(rwr.Rules, []byte(tt.manifest), Restore)
			require.NoError(t, err, "should rename validating webhook configuration")

			actual := string(resBytes)
			require.Equal(t, tt.expect, actual)
		})
	}
}
