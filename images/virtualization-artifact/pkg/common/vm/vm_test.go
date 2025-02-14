package vm

import (
	"testing"
)

func TestCalculateSockets(t *testing.T) {
	tests := []struct {
		cores    int
		expected int
	}{
		{8, 1},
		{16, 1},
		{17, 2},
		{32, 2},
		{33, 4},
		{36, 4},
		{64, 4},
		{72, 8},
		{128, 8},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			got := CalculateSockets(test.cores)
			if got != test.expected {
				t.Errorf("For %d cores, expected %d sockets, got %d", test.cores, test.expected, got)
			}
		})
	}
}
