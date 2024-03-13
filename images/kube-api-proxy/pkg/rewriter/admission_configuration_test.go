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
