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

package importer

import (
	"testing"

	metrics "github.com/deckhouse/virtualization/images/pvc-artifact/pkg/monitoring/metrics/pvc-importer"
)

func TestReportNbdcopyProgressUpdatesImportMetric(t *testing.T) {
	uid := "nbdcopy-progress"
	metrics.Progress(uid).Delete()

	reportNbdcopyProgress("0/100", uid)
	if got, err := metrics.Progress(uid).Get(); err != nil || got != 0 {
		t.Fatalf("expected 0 for zero progress, got %v err=%v", got, err)
	}

	reportNbdcopyProgress("25/100", uid)
	if got, err := metrics.Progress(uid).Get(); err != nil || got != 25 {
		t.Fatalf("expected 25, got %v err=%v", got, err)
	}

	reportNbdcopyProgress("10/100", uid)
	if got, err := metrics.Progress(uid).Get(); err != nil || got != 25 {
		t.Fatalf("expected progress to stay monotonic at 25, got %v err=%v", got, err)
	}

	reportNbdcopyProgress("100/100", uid)
	if got, err := metrics.Progress(uid).Get(); err != nil || got != 100 {
		t.Fatalf("expected 100, got %v err=%v", got, err)
	}
}

func TestReportNbdcopyProgressIgnoresInvalidLines(t *testing.T) {
	uid := "nbdcopy-progress-invalid"
	metrics.Progress(uid).Delete()

	reportNbdcopyProgress("(50.00/100%)", uid)
	reportNbdcopyProgress("50/100%", uid)
	reportNbdcopyProgress("", uid)

	if got, err := metrics.Progress(uid).Get(); err != nil || got != 0 {
		t.Fatalf("expected 0 for invalid lines, got %v err=%v", got, err)
	}
}
