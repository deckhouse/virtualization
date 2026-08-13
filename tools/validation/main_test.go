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

package main

import "testing"

func Test_resolveDiffRange(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		expected string
	}{
		{
			name:     "local run without CI environment",
			env:      map[string]string{},
			expected: "origin/main...",
		},
		{
			name: "merge request into main",
			env: map[string]string{
				"CI_MERGE_REQUEST_TARGET_BRANCH_NAME": "main",
				"CI_COMMIT_BEFORE_SHA":                zeroSHA,
			},
			expected: "origin/main...",
		},
		{
			name: "merge request into a release branch",
			env: map[string]string{
				"CI_MERGE_REQUEST_TARGET_BRANCH_NAME": "release-1.10",
				"CI_COMMIT_BEFORE_SHA":                zeroSHA,
			},
			expected: "origin/release-1.10...",
		},
		{
			name: "push pipeline validates the pushed commits",
			env: map[string]string{
				"CI_COMMIT_BRANCH":     "release-1.10",
				"CI_COMMIT_BEFORE_SHA": "9b55a3ce7051339cb36d5d1f5cffc0eb68c4e2e6",
			},
			expected: "9b55a3ce7051339cb36d5d1f5cffc0eb68c4e2e6..HEAD",
		},
		{
			name: "push pipeline for a new branch has no previous commit",
			env: map[string]string{
				"CI_COMMIT_BRANCH":     "feat/vm/new-branch",
				"CI_COMMIT_BEFORE_SHA": zeroSHA,
			},
			expected: "origin/main...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := resolveDiffRange(func(key string) string { return tt.env[key] })
			if actual != tt.expected {
				t.Errorf("Expect '%s', got '%s'", tt.expected, actual)
			}
		})
	}
}
