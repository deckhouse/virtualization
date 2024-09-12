package vmop

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
		phase = virtv2.VMOPPhasePending
	}
	phases := []struct {
		value bool
		name  string
	}{
		{phase == virtv2.VMOPPhasePending, string(virtv2.VMOPPhasePending)},
		{phase == virtv2.VMOPPhaseInProgress, string(virtv2.VMOPPhaseInProgress)},
		{phase == virtv2.VMOPPhaseCompleted, string(virtv2.VMOPPhaseCompleted)},
		{phase == virtv2.VMOPPhaseFailed, string(virtv2.VMOPPhaseFailed)},
		{phase == virtv2.VMOPPhaseTerminating, string(virtv2.VMOPPhaseTerminating)},
	}

	for _, p := range phases {
		s.defaultUpdate(MetricVMOPStatusPhase,
			util.BoolFloat64(p.value), m, p.name)
	}
}

func (s *scraper) defaultUpdate(descName string, value float64, m *dataMetric, labels ...string) {
	desc := vmopMetrics[descName]
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
