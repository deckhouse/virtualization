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

package humanize_bytes //nolint:stylecheck,nolintlint

import (
	"fmt"
	"math"
)

func HumanizeIBytes(s uint64) string {
	sizes := []string{"B", "Ki", "Mi", "Gi", "Ti", "Pi", "Ei"}
	return humanateBytes(s, 1024, sizes)
}

func humanateBytes(s uint64, base float64, sizes []string) string {
	if s < 10 {
		return fmt.Sprintf("%dB", s)
	}
	e := math.Floor(logn(float64(s), base))

	suffix := sizes[int(e)]
	val := math.Floor(float64(s)/math.Pow(base, e)*10+0.5) / 10
	_, frac := math.Modf(val)
	f := "%.0f%s"

	if frac > 0 {
		f = "%.1f%s"
	}

	return fmt.Sprintf(f, val, suffix)
}

func logn(n, b float64) float64 {
	return math.Log(n) / math.Log(b)
}
