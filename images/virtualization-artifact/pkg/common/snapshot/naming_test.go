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

package snapshot

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/types"
)

func TestGetVMSnapshotSecretName(t *testing.T) {
	testUID := types.UID("12345678-1234-1234-1234-123456789012")

	tests := []struct {
		name        string
		inputName   string
		expectedLen int
		hasPrefix   bool
		hasSuffix   bool
	}{
		{
			name:        "short name",
			inputName:   "my-snapshot",
			expectedLen: len("d8v-vms-my-snapshot-") + 36,
			hasPrefix:   true,
			hasSuffix:   true,
		},
		{
			name:        "very long name",
			inputName:   strings.Repeat("a", 300),
			expectedLen: 253,
			hasPrefix:   true,
			hasSuffix:   true,
		},
		{
			name:        "empty name",
			inputName:   "",
			expectedLen: len("d8v-vms--") + 36,
			hasPrefix:   true,
			hasSuffix:   true,
		},
		{
			name:        "name at limit",
			inputName:   strings.Repeat("b", 208),
			expectedLen: 253,
			hasPrefix:   true,
			hasSuffix:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetVMSnapshotSecretName(tt.inputName, testUID)

			if len(result) != tt.expectedLen {
				t.Errorf("expected length %d, got %d (result: %s)", tt.expectedLen, len(result), result)
			}

			if tt.hasPrefix && !strings.HasPrefix(result, "d8v-vms-") {
				t.Errorf("expected prefix 'd8v-vms-', got %s", result)
			}

			if tt.hasSuffix && !strings.HasSuffix(result, string(testUID)) {
				t.Errorf("expected suffix '%s', got %s", testUID, result)
			}

			if len(result) > maxResourceNameLength {
				t.Errorf("result exceeds max length: %d > %d", len(result), maxResourceNameLength)
			}
		})
	}
}

func TestGetVDSnapshotVolumeSnapshotName(t *testing.T) {
	testUID := types.UID("87654321-4321-4321-4321-210987654321")

	tests := []struct {
		name        string
		inputName   string
		expectedLen int
		hasPrefix   bool
		hasSuffix   bool
	}{
		{
			name:        "short name",
			inputName:   "my-disk-snapshot",
			expectedLen: len("d8v-vds-my-disk-snapshot-") + 36,
			hasPrefix:   true,
			hasSuffix:   true,
		},
		{
			name:        "very long name",
			inputName:   strings.Repeat("c", 300),
			expectedLen: 253,
			hasPrefix:   true,
			hasSuffix:   true,
		},
		{
			name:        "empty name",
			inputName:   "",
			expectedLen: len("d8v-vds--") + 36,
			hasPrefix:   true,
			hasSuffix:   true,
		},
		{
			name:        "name at limit",
			inputName:   strings.Repeat("d", 208),
			expectedLen: 253,
			hasPrefix:   true,
			hasSuffix:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetVDSnapshotVolumeSnapshotName(tt.inputName, testUID)

			if len(result) != tt.expectedLen {
				t.Errorf("expected length %d, got %d (result: %s)", tt.expectedLen, len(result), result)
			}

			if tt.hasPrefix && !strings.HasPrefix(result, "d8v-vds-") {
				t.Errorf("expected prefix 'd8v-vds-', got %s", result)
			}

			if tt.hasSuffix && !strings.HasSuffix(result, string(testUID)) {
				t.Errorf("expected suffix '%s', got %s", testUID, result)
			}

			if len(result) > maxResourceNameLength {
				t.Errorf("result exceeds max length: %d > %d", len(result), maxResourceNameLength)
			}
		})
	}
}

func TestGetLegacyVMSnapshotSecretName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple name",
			input:    "my-snapshot",
			expected: "my-snapshot",
		},
		{
			name:     "long name",
			input:    strings.Repeat("a", 300),
			expected: strings.Repeat("a", 300),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetLegacyVMSnapshotSecretName(tt.input)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestGetLegacyVDSnapshotVolumeSnapshotName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple name",
			input:    "my-disk-snapshot",
			expected: "my-disk-snapshot",
		},
		{
			name:     "long name",
			input:    strings.Repeat("b", 300),
			expected: strings.Repeat("b", 300),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetLegacyVDSnapshotVolumeSnapshotName(tt.input)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestTruncateName(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		maxLength int
		expected  string
	}{
		{
			name:      "no truncation needed",
			input:     "short",
			maxLength: 10,
			expected:  "short",
		},
		{
			name:      "truncation needed",
			input:     "very-long-name",
			maxLength: 5,
			expected:  "very-",
		},
		{
			name:      "exact length",
			input:     "exact",
			maxLength: 5,
			expected:  "exact",
		},
		{
			name:      "empty string",
			input:     "",
			maxLength: 10,
			expected:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateName(tt.input, tt.maxLength)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
			if len(result) > tt.maxLength {
				t.Errorf("result exceeds max length: %d > %d", len(result), tt.maxLength)
			}
		})
	}
}
