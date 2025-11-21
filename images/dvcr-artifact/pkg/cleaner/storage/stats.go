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

package storage

import (
	"fmt"
	"math"
	"strconv"
)

type FSInfo struct {
	Total     uint64
	Available uint64
}

func HumanizeQuantity(q uint64) string {
	suffixes := []string{"B", "Ki", "Mi", "Gi", "Ti", "Pi", "Ei"}
	return humanizeQuantity(q, 1024, suffixes)
}

func humanizeQuantity(s uint64, base float64, suffixes []string) string {
	if s < 1200 {
		return strconv.FormatUint(s, 10)
	}

	e := math.Floor(logn(float64(s), base))

	suffix := suffixes[int(e)]
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
