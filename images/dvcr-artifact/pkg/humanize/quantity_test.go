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

package humanize

import (
	"math"
	"testing"
)

func Test_Humanize(t *testing.T) {
	tests := []struct {
		in  int64
		out string
	}{
		{
			0,
			"0",
		},
		{
			10,
			"10",
		},
		{
			1024,
			"1.00Ki",
		},
		{
			1024 * 1024,
			"1.00Mi",
		},
		{
			2*1024*1024 + 2*1024,
			"2.00Mi",
		},
		{
			2*1024*1024 + 20*1024,
			"2.02Mi",
		},
		{
			2*1024*1024 + 200*1024,
			"2.20Mi",
		},
		{
			32*1024*1024 + 2*1024,
			"32.0Mi",
		},
		{
			32*1024*1024 + 20*1024,
			"32.0Mi",
		},
		{
			32*1024*1024 + 200*1024,
			"32.2Mi",
		},
		{
			432*1024*1024 + 2*1024,
			"432Mi",
		},
		{
			432*1024*1024 + 20*1024,
			"432Mi",
		},
		{
			432*1024*1024 + 200*1024,
			"432Mi",
		},
		{
			1024 * 1024 * 1024,
			"1.00Gi",
		},
		{
			math.MaxInt64,
			"7.10Zi",
		},
		{
			-(1024 * 1024),
			"-1.00Mi",
		},
		{
			-(2*1024*1024 + 2*1024),
			"-2.00Mi",
		},
		{
			-(2*1024*1024 + 20*1024),
			"-2.02Mi",
		},
		{
			-(2*1024*1024 + 200*1024),
			"-2.20Mi",
		},
		{
			-(32*1024*1024 + 2*1024),
			"-32.0Mi",
		},
		{
			-(32*1024*1024 + 20*1024),
			"-32.0Mi",
		},
		{
			-(32*1024*1024 + 200*1024),
			"-32.2Mi",
		},
		{
			-(432*1024*1024 + 2*1024),
			"-432Mi",
		},
		{
			-(432*1024*1024 + 20*1024),
			"-432Mi",
		},
		{
			-(432*1024*1024 + 200*1024),
			"-432Mi",
		},
		{
			-(1024 * 1024 * 1024),
			"-1.00Gi",
		},
		{
			math.MinInt64,
			"-7.10Zi",
		},
	}

	for _, tt := range tests {
		t.Run(tt.out, func(t *testing.T) {
			actual := humanizeQuantity4(tt.in, BIBase)
			if tt.out != actual {
				t.Fatalf("expect '%s' quantity for %d, got '%s'", tt.out, tt.in, actual)
			}
		})
	}
}
