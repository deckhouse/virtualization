package common

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
)

func ScalePercentage(percent string, low, high float64) string {
	pctVal := ExtractPercentageFloat(percent)
	if math.IsNaN(pctVal) {
		return percent
	}

	scaled := pctVal*((high-low)/100) + low
	return fmt.Sprintf("%.1f%%", scaled)
}

var possibleFloatRe = regexp.MustCompile(`[0-9eE\-+.]+`)

func ExtractPercentageFloat(in string) float64 {
	parts := possibleFloatRe.FindStringSubmatch(in)
	if len(parts) == 0 {
		return math.NaN()
	}
	v, err := strconv.ParseFloat(parts[0], 32)
	if err != nil {
		return math.NaN()
	}
	return v
}
