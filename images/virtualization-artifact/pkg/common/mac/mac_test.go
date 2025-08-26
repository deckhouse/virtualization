/*
Copyright 2025 Flant JSC

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

package mac

import (
	"testing"
)

func TestGenerateOUI(t *testing.T) {
	testCases := []struct {
		name      string
		clusterID string
		expected  string
	}{
		{
			name:      "Valid UUID with '6' at second position",
			clusterID: "b6f6f9e5-9c5c-4e8c-8b8b-8c8b8c8b8c8b",
			expected:  "b6:f6:f9",
		},
		{
			name:      "Valid UUID with '2' at second position",
			clusterID: "d2f6f9e5-9c5c-4e8c-8b8b-8c8b8c8b8c8b",
			expected:  "d2:f6:f9",
		},
		{
			name:      "Valid UUID with 'a' at second position",
			clusterID: "aaf6f9e5-9c5c-4e8c-8b8b-8c8b8c8b8c8b",
			expected:  "aa:f6:f9",
		},
		{
			name:      "Valid UUID with 'e' at second position",
			clusterID: "eef6f9e5-9c5c-4e8c-8b8b-8c8b8c8b8c8b",
			expected:  "ee:f6:f9",
		},
		{
			name:      "UUID is too short",
			clusterID: "bcf6f9e5-9c5c-4e8c",
			expected:  "",
		},
		{
			name:      "UUID with invalid characters",
			clusterID: "bcf6f9e5-9c5c-4e8c-8b8b-8c8b8c8b8c8g",
			expected:  "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := GenerateOUI(tc.clusterID)
			if result != tc.expected {
				t.Errorf("GenerateOUI(%s) got %s, want %s", tc.clusterID, result, tc.expected)
			}
		})
	}
}
