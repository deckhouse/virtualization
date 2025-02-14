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

package vm

import (
	"testing"
)

func TestCalculateCoresAndSockets(t *testing.T) {
	tests := []struct {
		desiredCores int
		sockets      int
		cores        int
	}{
		{8, 1, 8},
		{16, 1, 1},
		{17, 2, 9},
		{32, 2, 16},
		{33, 4, 9},
		{36, 4, 9},
		{64, 4, 16},
		{72, 8, 9},
		{128, 8, 16},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			sockets, cores := CalculateCoresAndSockets(test.desiredCores)
			if cores != test.cores && sockets != test.sockets {
				t.Errorf("For %d cores, expected %d sockets and %d cores, got  %d sockets and %d cores", test.desiredCores, test.sockets, test.cores, sockets, cores)
			}
		})
	}
}
