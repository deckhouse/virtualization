package vmbda

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
	s.updateVMBDAStatusPhaseMetrics(m)
}

func (s *scraper) updateVMBDAStatusPhaseMetrics(m *dataMetric) {
	phase := m.Phase
	if phase == "" {
		phase = virtv2.BlockDeviceAttachmentPhasePending
	}
	phases := []struct {
		value bool
		name  string
	}{
		{phase == virtv2.BlockDeviceAttachmentPhasePending, string(virtv2.BlockDeviceAttachmentPhasePending)},
		{phase == virtv2.BlockDeviceAttachmentPhaseInProgress, string(virtv2.BlockDeviceAttachmentPhaseInProgress)},
		{phase == virtv2.BlockDeviceAttachmentPhaseAttached, string(virtv2.BlockDeviceAttachmentPhaseAttached)},
		{phase == virtv2.BlockDeviceAttachmentPhaseFailed, string(virtv2.BlockDeviceAttachmentPhaseFailed)},
		{phase == virtv2.BlockDeviceAttachmentPhaseTerminating, string(virtv2.BlockDeviceAttachmentPhaseTerminating)},
	}

	for _, p := range phases {
		s.defaultUpdate(MetricVMBDAStatusPhase,
			util.BoolFloat64(p.value), m, p.name)
	}
}

func (s *scraper) defaultUpdate(descName string, value float64, m *dataMetric, labels ...string) {
	desc := vmbdaMetrics[descName]
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
