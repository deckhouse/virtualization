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

package version

import (
	"testing"

	"github.com/blang/semver/v4"
)

func TestNormalizeSemVerRange(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "two-component version gets patch appended",
			input:    ">= 1.74",
			expected: ">= 1.74.0",
		},
		{
			name:     "three-component version stays unchanged",
			input:    ">= 1.74.2",
			expected: ">= 1.74.2",
		},
		{
			name:     "multiple two-component versions in compound range",
			input:    ">= 1.74 < 2.0",
			expected: ">= 1.74.0 < 2.0.0",
		},
		{
			name:     "mixed two and three component versions",
			input:    ">= 1.74 < 2.0.1",
			expected: ">= 1.74.0 < 2.0.1",
		},
		{
			name:     "already valid range unchanged",
			input:    ">= 0.0.0",
			expected: ">= 0.0.0",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeSemVerRange(tt.input)
			if got != tt.expected {
				t.Errorf("NormalizeSemVerRange(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestNormalizeSemVerRange_ParseableByBlangSemver(t *testing.T) {
	ranges := []string{
		">= 1.74",
		">= 1.74.2",
		">= 1.74 < 2.0",
		">= 0.0.0",
	}

	for _, r := range ranges {
		t.Run(r, func(t *testing.T) {
			normalized := NormalizeSemVerRange(r)
			_, err := semver.ParseRange(normalized)
			if err != nil {
				t.Errorf("NormalizeSemVerRange(%q) = %q, which is not parseable: %v", r, normalized, err)
			}
		})
	}
}
