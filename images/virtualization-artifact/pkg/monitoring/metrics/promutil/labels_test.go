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

package promutil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWrapPrometheusLabels(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		expected map[string]string
	}{
		{
			name: "should resolve conflicted labels",
			labels: map[string]string{
				"key-1": "value1",
				"key_1": "value1",
				"key-2": "value2",
			},
			expected: map[string]string{
				"label_key_1_conflict1": "value1",
				"label_key_1_conflict2": "value1",
				"label_key_2":           "value2",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wrapped := WrapPrometheusLabels(test.labels, "label", nil)
			require.Equal(t, test.expected, wrapped)
		})
	}
}
