package common

import (
	"fmt"
	"testing"
)

func Test_ScalePercent(t *testing.T) {
	tests := []struct {
		in     string
		low    float64
		high   float64
		expect string
	}{
		{
			"100.0%",
			0.0,
			50.0,
			"50.0%",
		},
		{
			"1%",
			0.0,
			50.0,
			"0.5%", // 0 + 1/2
		},
		{
			"0%",
			0.0,
			50.0,
			"0.0%",
		},
		{
			"0%",
			50.0,
			100.0,
			"50.0%",
		},
		{
			"66.4%",
			50.0,
			100.0,
			"83.2%", // 50 + 33.2
		},
		{
			"100%",
			50.0,
			100.0,
			"100.0%",
		},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s -> (%.1f ; %.1f)", tt.in, tt.low, tt.high), func(t *testing.T) {
			out := ScalePercentage(tt.in, tt.low, tt.high)
			if out != tt.expect {
				t.Fatalf("expect %s scaled into (%.1f; %.1f) is %s, got %s", tt.in, tt.low, tt.high, tt.expect, out)
			}
		})
	}
}
