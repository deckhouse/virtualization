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

package image

import (
	"testing"

	metrics "kubevirt.io/containerized-data-importer/pkg/monitoring/metrics/pvc-importer"
)

func TestReportProgressFullUpdatesImportMetric(t *testing.T) {
	uid := "report-progress-full"
	ownerUID = uid
	metrics.Progress(uid).Delete()

	reportProgressFull("(0.00/100%)")
	if got, err := metrics.Progress(uid).Get(); err != nil || got != 0 {
		t.Fatalf("expected 0 after zero qemu progress, got %v err=%v", got, err)
	}

	reportProgressFull("(50.00/100%)")
	if got, err := metrics.Progress(uid).Get(); err != nil || got != 50 {
		t.Fatalf("expected 50, got %v err=%v", got, err)
	}

	reportProgressFull("(75.00/100%)")
	if got, err := metrics.Progress(uid).Get(); err != nil || got != 75 {
		t.Fatalf("expected 75, got %v err=%v", got, err)
	}

	reportProgressFull("(25.00/100%)")
	if got, err := metrics.Progress(uid).Get(); err != nil || got != 75 {
		t.Fatalf("expected progress to stay monotonic at 75, got %v err=%v", got, err)
	}
}

func TestReportProgressScalesConvertPhaseIntoUpperHalf(t *testing.T) {
	uid := "report-progress-convert"
	ownerUID = uid
	metrics.Progress(uid).Delete()

	reportProgress("(99.99/100%)")
	if got, err := metrics.Progress(uid).Get(); err != nil || got < 99.9 {
		t.Fatalf("expected ~100 at end of convert phase, got %v err=%v", got, err)
	}

	metrics.Progress(uid).Delete()
	reportProgress("(50.00/100%)")
	if got, err := metrics.Progress(uid).Get(); err != nil || got != 75 {
		t.Fatalf("expected 75 for halfway qemu convert, got %v err=%v", got, err)
	}
}
