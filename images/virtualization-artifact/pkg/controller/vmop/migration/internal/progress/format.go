/*
Copyright 2026 Flant JSC

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

package progress

import (
	"fmt"
	"math"

	commonpercent "github.com/deckhouse/virtualization-controller/pkg/common/percent"
)

func FormatPercent(v int32) string {
	return fmt.Sprintf("%d%%", v)
}

func ParsePercent(v string) int32 {
	if v == "" {
		return SyncRangeMin
	}

	parsed := commonpercent.ExtractPercentageFloat(v)
	if math.IsNaN(parsed) {
		return SyncRangeMin
	}

	return int32(parsed)
}
