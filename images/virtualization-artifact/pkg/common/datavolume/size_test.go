package datavolume

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestAdjustPVCSize(t *testing.T) {
	tests := []struct {
		name string
		in   int64
		out  int64
	}{
		{
			"less than 512Mi",
			100 * 1024 * 1024,
			126 * 1024 * 1024, // 25% + 1Mi
		},
		{
			"less than 4096Mi",
			2000 * 1024 * 1024,
			2301 * 1024 * 1024, // 15% + 1Mi
		},
		{
			"more than 4096Mi",
			10000 * 1024 * 1024,
			11001 * 1024 * 1024, // 10% + 1Mi
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := AdjustPVCSize(*resource.NewQuantity(tt.in, resource.BinarySI))
			require.Equal(t, tt.out, res.Value())
		})
	}
}
