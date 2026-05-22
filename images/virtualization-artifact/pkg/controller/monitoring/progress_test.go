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

// Test_ExtractProgress_CDIImporter verifies the parser accepts the
// cdi-importer's progress metric name and returns the value as the pod-local
// progress. cdi-importer does not emit registry_*_speed series, so download
// speed remains zero in this case.
func Test_ExtractProgress_CDIImporter(t *testing.T) {
	p, err := extractProgress(`
kubevirt_cdi_import_progress_total{ownerUID="22"} 73.42
`, `22`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p == nil {
		t.Fatalf("expected progress, got nil")
	}
	if p.ProgressRaw() != 73.42 {
		t.Fatalf("expected 73.42, got %v", p.ProgressRaw())
	}
	if p.AvgSpeedRaw() != 0 || p.CurSpeedRaw() != 0 {
		t.Fatalf("expected zero speed (cdi-importer does not emit speed), got avg=%d cur=%d", p.AvgSpeedRaw(), p.CurSpeedRaw())
	}
}

// Test_ExtractProgress_DVCRTakesPrecedence covers the case where both metric
// families happen to be present on the same scrape (e.g. mixed report from a
// previous run). The dvcr-importer's registry_progress name is listed first in
// the alternation, so it wins; speeds from the same family are picked up too.
func Test_ExtractProgress_DVCRTakesPrecedence(t *testing.T) {
	p, err := extractProgress(`
registry_progress{ownerUID="33"} 47.6
registry_average_speed{ownerUID="33"} 1.0e+6
registry_current_speed{ownerUID="33"} 2.0e+6
kubevirt_cdi_import_progress_total{ownerUID="33"} 99.0
`, `33`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p == nil {
		t.Fatalf("expected progress, got nil")
	}
	if p.ProgressRaw() != 47.6 {
		t.Fatalf("expected 47.6, got %v", p.ProgressRaw())
	}
	if p.AvgSpeedRaw() != 1_000_000 || p.CurSpeedRaw() != 2_000_000 {
		t.Fatalf("expected avg=1e6 cur=2e6, got avg=%d cur=%d", p.AvgSpeedRaw(), p.CurSpeedRaw())
	}
}
