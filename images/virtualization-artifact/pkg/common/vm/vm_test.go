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
