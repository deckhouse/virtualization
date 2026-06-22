package importer

import (
	"testing"

	metrics "kubevirt.io/containerized-data-importer/pkg/monitoring/metrics/pvc-importer"
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
