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

package monitoring

import (
	"testing"
)

func Test_Humanize(t *testing.T) {
	p, _ := extractProgress(`
registry_progress{ownerUID="11"} 12.34
registry_average_speed{ownerUID="11"} 1.234532345432e+6
registry_current_speed{ownerUID="11"} 2.345632345432e+6
`, `11`)
	if p.AvgSpeed() != `1.2Mi/s` {
		t.Fatalf("%s is not expected human readable value for raw value %v", p.AvgSpeed(), p.AvgSpeedRaw())
	}

	if p.CurSpeed() != `2.2Mi/s` {
		t.Fatalf("%s is not expected human readable value for raw value %v", p.CurSpeed(), p.CurSpeedRaw())
	}
}
