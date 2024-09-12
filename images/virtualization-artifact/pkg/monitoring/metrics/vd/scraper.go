package vd

import (
	"fmt"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/deckhouse/virtualization-controller/pkg/util"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func newScraper(ch chan<- prometheus.Metric, log *slog.Logger) *scraper {
	return &scraper{ch: ch, log: log}
}

type scraper struct {
	ch  chan<- prometheus.Metric
	log *slog.Logger
}

func (s *scraper) Report(m *dataMetric) {
	s.updateDiskStatusPhaseMetrics(m)
}

func (s *scraper) updateDiskStatusPhaseMetrics(m *dataMetric) {
	phase := m.Phase
	if phase == "" {
		phase = virtv2.DiskPending
	}
	phases := []struct {
		value bool
		name  string
	}{
		{phase == virtv2.DiskPending, string(virtv2.DiskPending)},
		{phase == virtv2.DiskWaitForUserUpload, string(virtv2.DiskWaitForUserUpload)},
		{phase == virtv2.DiskWaitForFirstConsumer, string(virtv2.DiskWaitForFirstConsumer)},
		{phase == virtv2.DiskProvisioning, string(virtv2.DiskProvisioning)},
		{phase == virtv2.DiskFailed, string(virtv2.DiskFailed)},
		{phase == virtv2.DiskLost, string(virtv2.DiskLost)},
		{phase == virtv2.DiskReady, string(virtv2.DiskReady)},
		{phase == virtv2.DiskResizing, string(virtv2.DiskResizing)},
		{phase == virtv2.DiskTerminating, string(virtv2.DiskTerminating)},
	}

	for _, p := range phases {
		s.defaultUpdate(MetricDiskStatusPhase,
			util.BoolFloat64(p.value), m, p.name)
	}
}

func (s *scraper) defaultUpdate(descName string, value float64, m *dataMetric, labels ...string) {
	desc := diskMetrics[descName]
	metric, err := prometheus.NewConstMetric(
		desc,
		prometheus.GaugeValue,
		value,
		WithBaseLabelsByMetric(m, labels...)...,
	)
	if err != nil {
		s.log.Warn(fmt.Sprintf("Error creating the new const dataMetric for %s: %s", desc, err))
		return
	}
	s.ch <- metric
}
