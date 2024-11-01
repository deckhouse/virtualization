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
			"several groups",
			`{"apiGroups":["original.group.io","other.group.io"],
"resources": ["*"],
"verbs": ["watch", "list", "create"]
}`,
			`{"apiGroups":["prefixed.resources.group.io","other.prefixed.resources.group.io"],
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
			resBytes, err := RenameResourceRule(rwr.Rules, []byte(tt.rule))
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
		{
			"only group",
			`{"apiGroups":["prefixed.resources.group.io"],
		"resources": ["*"],
		"verbs": ["watch", "list", "create"]
		}`,
			`{"apiGroups":["original.group.io"],
		"resources": ["*"],
		"verbs": ["watch", "list", "create"]
		}`,
		},
		{
			"several groups",
			`{"apiGroups":["prefixed.resources.group.io","other.prefixed.resources.group.io"],
		"resources": ["*"],
		"verbs": ["watch", "list", "create"]
		}`,
			`{"apiGroups":["original.group.io","other.group.io"],
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
			`{"apiGroups":[""], "resources":["pods","configmaps"], "verbs":["create"]}`,
			`{"apiGroups":[""], "resources":["pods","configmaps"], "verbs":["create"]}`,
		},
	}

	rwr := createTestRewriter()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resBytes, err := RestoreResourceRule(rwr.Rules, []byte(tt.rule))
			require.NoError(t, err, "should rename rule")

			actual := string(resBytes)
			require.Equal(t, tt.expect, actual)
		})
	}
}
