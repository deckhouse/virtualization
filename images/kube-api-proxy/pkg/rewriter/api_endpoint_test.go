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
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseAPIEndpoint(t *testing.T) {

	tests := []struct {
		name   string
		path   string
		expect *APIEndpoint
	}{
		{
			"root",
			"/",
			&APIEndpoint{
				IsRoot: true,
			},
		},

		// Core resources.
		{
			"core apiversions",
			"/api",
			&APIEndpoint{
				IsCore: true,
				Prefix: CorePrefix,
			},
		},
		{
			"core apiresourcelist",
			"/api/v1",
			&APIEndpoint{
				IsCore:  true,
				Prefix:  CorePrefix,
				Version: "v1",
			},
		},
		{
			"core deploymentlist",
			"/api/v1/deployments",
			&APIEndpoint{
				IsCore:       true,
				Prefix:       CorePrefix,
				Version:      "v1",
				ResourceType: "deployments",
			},
		},
		{
			"core deployment dy name",
			"/api/v1/deployments/deployname",
			&APIEndpoint{
				IsCore:       true,
				Prefix:       CorePrefix,
				Version:      "v1",
				ResourceType: "deployments",
				Name:         "deployname",
			},
		},
		{
			"core deployment status",
			"/api/v1/deployments/deployname/status",
			&APIEndpoint{
				IsCore:       true,
				Prefix:       CorePrefix,
				Version:      "v1",
				ResourceType: "deployments",
				Name:         "deployname",
				Subresource:  "status",
			},
		},
		{
			"core deployments in nsname",
			"/api/v1/namespaces/nsname/deployments",
			&APIEndpoint{
				IsCore:       true,
				Prefix:       CorePrefix,
				Version:      "v1",
				ResourceType: "deployments",
				Namespace:    "nsname",
			},
		},
		{
			"core deployment in nsname by name",
			"/api/v1/namespaces/nsname/deployments/deployname",
			&APIEndpoint{
				IsCore:       true,
				Prefix:       CorePrefix,
				Version:      "v1",
				ResourceType: "deployments",
				Namespace:    "nsname",
				Name:         "deployname",
			},
		},
		{
			"core deployment status in nsname",
			"/api/v1/namespaces/nsname/deployments/deployname/status",
			&APIEndpoint{
				IsCore:       true,
				Prefix:       CorePrefix,
				Version:      "v1",
				ResourceType: "deployments",
				Namespace:    "nsname",
				Name:         "deployname",
				Subresource:  "status",
			},
		},

		// Custom resources.
		{
			"apigrouplist",
			"/apis",
			&APIEndpoint{
				Prefix: APIsPrefix,
			},
		},
		{
			"apigroup",
			"/apis/group.io",
			&APIEndpoint{
				Prefix: APIsPrefix,
				Group:  "group.io",
			},
		},
		{
			"apiresourcelist",
			"/apis/group.io/v1",
			&APIEndpoint{
				Prefix:  APIsPrefix,
				Group:   "group.io",
				Version: "v1",
			},
		},
		{
			"someresourceslist",
			"/apis/group.io/v1/someresources",
			&APIEndpoint{
				Prefix:       APIsPrefix,
				Group:        "group.io",
				Version:      "v1",
				ResourceType: "someresources",
			},
		},
		{
			"someresource by name",
			"/apis/group.io/v1/someresources/srname",
			&APIEndpoint{
				Prefix:       APIsPrefix,
				Group:        "group.io",
				Version:      "v1",
				ResourceType: "someresources",
				Name:         "srname",
			},
		},
		{
			"someresource status",
			"/apis/group.io/v1/someresources/srname/status",
			&APIEndpoint{
				Prefix:       APIsPrefix,
				Group:        "group.io",
				Version:      "v1",
				ResourceType: "someresources",
				Name:         "srname",
				Subresource:  "status",
			},
		},
		{
			"someresources in nsname",
			"/apis/group.io/v1/namespaces/nsname/someresources",
			&APIEndpoint{
				Prefix:       APIsPrefix,
				Group:        "group.io",
				Version:      "v1",
				Namespace:    "nsname",
				ResourceType: "someresources",
			},
		},
		{
			"someresource in nsname by name",
			"/apis/group.io/v1/namespaces/nsname/someresources/srname",
			&APIEndpoint{
				Prefix:       APIsPrefix,
				Group:        "group.io",
				Version:      "v1",
				Namespace:    "nsname",
				ResourceType: "someresources",
				Name:         "srname",
			},
		},
		{
			"someresource status in nsname",
			"/apis/group.io/v1/namespaces/nsname/someresources/srname/status",
			&APIEndpoint{
				Prefix:       APIsPrefix,
				Group:        "group.io",
				Version:      "v1",
				Namespace:    "nsname",
				ResourceType: "someresources",
				Name:         "srname",
				Subresource:  "status",
			},
		},

		// CRDs
		{
			"crd list",
			"/apis/apiextensions.k8s.io/v1/customresourcedefinitions",
			&APIEndpoint{
				IsCRD:        true,
				Prefix:       APIsPrefix,
				Group:        "apiextensions.k8s.io",
				Version:      "v1",
				ResourceType: "customresourcedefinitions",
			},
		},
		{
			"crd by name",
			"/apis/apiextensions.k8s.io/v1/customresourcedefinitions/crname",
			&APIEndpoint{
				IsCRD:        true,
				Prefix:       APIsPrefix,
				Group:        "apiextensions.k8s.io",
				Version:      "v1",
				ResourceType: "customresourcedefinitions",
				Name:         "crname",
			},
		},
		{
			"crd status",
			"/apis/apiextensions.k8s.io/v1/customresourcedefinitions/crname/status",
			&APIEndpoint{
				IsCRD:        true,
				Prefix:       APIsPrefix,
				Group:        "apiextensions.k8s.io",
				Version:      "v1",
				ResourceType: "customresourcedefinitions",
				Name:         "crname",
				Subresource:  "status",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.path)
			require.NoError(t, err, "should parse path '%s'", tt.path)

			actual := ParseAPIEndpoint(u)
			if tt.expect == nil {
				require.Nil(t, actual, "expect not parse path '%s', got non-empty %+v", tt.path, actual)
			}

			if tt.expect != nil {
				require.NotNil(t, actual, "expect parse path '%s' to %+v, got nil", tt.path, tt.expect)

				// Flags.
				require.Equal(t, tt.expect.IsRoot, actual.IsRoot, "IsRoot")
				require.Equal(t, tt.expect.IsCore, actual.IsCore, "IsCore")
				require.Equal(t, tt.expect.IsCRD, actual.IsCRD, "IsCRD")

				// Parts.
				require.Equal(t, tt.expect.Prefix, actual.Prefix, "Prefix")
				require.Equal(t, tt.expect.Group, actual.Group, "Group")
				require.Equal(t, tt.expect.Version, actual.Version, "Version")
				require.Equal(t, tt.expect.ResourceType, actual.ResourceType, "ResourceType")
				require.Equal(t, tt.expect.Name, actual.Name, "Name")
				require.Equal(t, tt.expect.Subresource, actual.Subresource, "Subresource")
				require.Equal(t, tt.expect.Namespace, actual.Namespace, "Namespace")
			}
		})
	}
}
