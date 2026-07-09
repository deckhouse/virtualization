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

package percent

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
	return Format(scaled)
}

// Format renders an import-progress percentage for display. The boundary values
// 0 and 100 are rendered without a fractional part ("0%", "100%"); every other
// value keeps a single decimal ("32.1%", "50.0%"). This keeps the reported
// progress consistent (e.g. always "50.0%" instead of a mix of "50%" and
// "50.0%").
func Format(value float64) string {
	switch value {
	case 0:
		return "0%"
	case 100:
		return "100%"
	default:
		return fmt.Sprintf("%.1f%%", value)
	}
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
