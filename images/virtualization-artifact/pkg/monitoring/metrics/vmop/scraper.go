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

package vmop

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/common"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func newScraper(ch chan<- prometheus.Metric, log *log.Logger) *scraper {
	return &scraper{ch: ch, log: log}
}

type scraper struct {
	ch  chan<- prometheus.Metric
	log *log.Logger
}

func (s *scraper) Report(m *dataMetric) {
	s.updateMetricVMOPStatusPhase(m)
}

func (s *scraper) updateMetricVMOPStatusPhase(m *dataMetric) {
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
			common.BoolFloat64(p.value), m, p.name)
	}
}

func (s *scraper) defaultUpdate(descName string, value float64, m *dataMetric, labels ...string) {
	info := vmopMetrics[descName]
	metric, err := prometheus.NewConstMetric(
		info.Desc,
		prometheus.GaugeValue,
		value,
		WithBaseLabelsByMetric(m, labels...)...,
	)
	if err != nil {
		s.log.Warn(fmt.Sprintf("Error creating the new const dataMetric for %s: %s", info.Desc, err))
		return
	}
	s.ch <- metric
}
