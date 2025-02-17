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
		{1, 1, 1},
		{2, 1, 2},
		{3, 1, 3},
		{4, 1, 4},
		{5, 1, 5},
		{6, 1, 6},
		{7, 1, 7},
		{8, 1, 8},
		{9, 1, 9},
		{10, 1, 10},
		{11, 1, 11},
		{12, 1, 12},
		{13, 1, 13},
		{14, 1, 14},
		{15, 1, 15},
		{16, 1, 16},

		{18, 2, 9},
		{20, 2, 10},
		{22, 2, 11},
		{24, 2, 12},
		{26, 2, 13},
		{28, 2, 14},
		{30, 2, 15},
		{32, 2, 16},

		{36, 4, 9},
		{40, 4, 10},
		{44, 4, 11},
		{48, 4, 12},
		{52, 4, 13},
		{56, 4, 14},
		{60, 4, 15},
		{64, 4, 16},

		{72, 8, 9},
		{80, 8, 10},
		{88, 8, 11},
		{96, 8, 12},
		{104, 8, 13},
		{112, 8, 14},
		{120, 8, 15},
		{128, 8, 16},
		{136, 8, 17},
		{144, 8, 18},
		{152, 8, 19},
		{160, 8, 20},
		{168, 8, 21},
		{176, 8, 22},
		{184, 8, 23},
		{192, 8, 24},
		{200, 8, 25},
		{208, 8, 26},
		{216, 8, 27},
		{224, 8, 28},
		{232, 8, 29},
		{240, 8, 30},
		{248, 8, 31},
		{256, 8, 32},
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
