package rewriter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRenameRoleRule(t *testing.T) {

	tests := []struct {
		name   string
		rule   string
		expect string
	}{
		{
			"group and resources",
			`{"apiGroups":["original.group.io"],
"resources": ["someresources","someresources/finalizers","someresources/status"],
"verbs": ["watch", "list", "create"]
}`,
			`{"apiGroups":["prefixed.resources.group.io"],
"resources": ["prefixedsomeresources","prefixedsomeresources/finalizers","prefixedsomeresources/status"],
"verbs": ["watch", "list", "create"]
}`,
		},
		{
			"only resources",
			`{"apiGroups":["*"],
"resources": ["someresources","someresources/finalizers","someresources/status"],
"verbs": ["watch", "list", "create"]
}`,
			`{"apiGroups":["*"],
"resources": ["prefixedsomeresources","prefixedsomeresources/finalizers","prefixedsomeresources/status"],
"verbs": ["watch", "list", "create"]
}`,
		},
		{
			"only group",
			`{"apiGroups":["original.group.io"],
"resources": ["*"],
"verbs": ["watch", "list", "create"]
}`,
			`{"apiGroups":["prefixed.resources.group.io"],
"resources": ["*"],
"verbs": ["watch", "list", "create"]
}`,
		},
		{
			"allow all",
			`{"apiGroups":["*"], "resources":["*"], "verbs":["*"]}`,
			`{"apiGroups":["*"], "resources":["*"], "verbs":["*"]}`,
		},
		{
			"unknown group",
			`{"apiGroups":["unknown.group.io"], "resources":["someresources"], "verbs":["*"]}`,
			`{"apiGroups":["unknown.group.io"], "resources":["someresources"], "verbs":["*"]}`,
		},
		{
			"core resource",
			`{"apiGroups":[""], "resources":["pods"], "verbs":["create"]}`,
			`{"apiGroups":[""], "resources":["pods"], "verbs":["create"]}`,
		},
	}

	rwr := createTestRewriter()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resBytes, err := renameRoleRule(rwr.Rules, []byte(tt.rule))
			require.NoError(t, err, "should rename rule")

			actual := string(resBytes)
			require.Equal(t, tt.expect, actual)
		})
	}
}

func TestRestoreRoleRule(t *testing.T) {
	tests := []struct {
		name   string
		rule   string
		expect string
	}{
		{
			"group and resources",
			`{"apiGroups":["prefixed.resources.group.io"],
"resources": ["prefixedsomeresources","prefixedsomeresources/finalizers","prefixedsomeresources/status"],
"verbs": ["watch", "list", "create"]
}`,
			`{"apiGroups":["original.group.io"],
"resources": ["someresources","someresources/finalizers","someresources/status"],
"verbs": ["watch", "list", "create"]
}`,
		},
		{
			"only resources",
			`{"apiGroups":["*"],
"resources": ["prefixedsomeresources","prefixedsomeresources/finalizers","prefixedsomeresources/status"],
"verbs": ["watch", "list", "create"]
}`,
			`{"apiGroups":["*"],
"resources": ["someresources","someresources/finalizers","someresources/status"],
"verbs": ["watch", "list", "create"]
}`,
		},
		// Impossible to restore with current rules.
		//		{
		//			"only group",
		//			`{"apiGroups":["prefixed.resources.group.io"],
		//"resources": ["*"],
		//"verbs": ["watch", "list", "create"]
		//}`,
		//			`{"apiGroups":["original.group.io"],
		//"resources": ["*"],
		//"verbs": ["watch", "list", "create"]
		//}`,
		//		},
		{
			"allow all",
			`{"apiGroups":["*"], "resources":["*"], "verbs":["*"]}`,
			`{"apiGroups":["*"], "resources":["*"], "verbs":["*"]}`,
		},
		{
			"unknown group",
			`{"apiGroups":["unknown.group.io"], "resources":["someresources"], "verbs":["*"]}`,
			`{"apiGroups":["unknown.group.io"], "resources":["someresources"], "verbs":["*"]}`,
		},
		{
			"core resource",
			`{"apiGroups":[""], "resources":["pods","configmaps"], "verbs":["create"]}`,
			`{"apiGroups":[""], "resources":["pods","configmaps"], "verbs":["create"]}`,
		},
	}

	rwr := createTestRewriter()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resBytes, err := restoreRoleRule(rwr.Rules, []byte(tt.rule))
			require.NoError(t, err, "should rename rule")

			actual := string(resBytes)
			require.Equal(t, tt.expect, actual)
		})
	}
}
