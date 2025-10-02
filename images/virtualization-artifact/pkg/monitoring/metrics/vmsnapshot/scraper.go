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

package vmsnapshot

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func newScraper(ch chan<- prometheus.Metric, log *log.Logger) *scraper {
	return &scraper{ch: ch, log: log}
}

type scraper struct {
	ch  chan<- prometheus.Metric
	log *log.Logger
}

func (s *scraper) Report(m *dataMetric) {
	s.updateMetricVMSnapshotStatusPhase(m)
}

func (s *scraper) updateMetricVMSnapshotStatusPhase(m *dataMetric) {
	phase := m.Phase
	if phase == "" {
		phase = v1alpha2.VirtualMachineSnapshotPhasePending
	}
	phases := []struct {
		value bool
		name  string
	}{
		{phase == v1alpha2.VirtualMachineSnapshotPhasePending, string(v1alpha2.VirtualMachineSnapshotPhasePending)},
		{phase == v1alpha2.VirtualMachineSnapshotPhaseInProgress, string(v1alpha2.VirtualMachineSnapshotPhaseInProgress)},
		{phase == v1alpha2.VirtualMachineSnapshotPhaseReady, string(v1alpha2.VirtualMachineSnapshotPhaseReady)},
		{phase == v1alpha2.VirtualMachineSnapshotPhaseFailed, string(v1alpha2.VirtualMachineSnapshotPhaseFailed)},
		{phase == v1alpha2.VirtualMachineSnapshotPhaseTerminating, string(v1alpha2.VirtualMachineSnapshotPhaseTerminating)},
	}

	for _, p := range phases {
		s.defaultUpdate(MetricVMSnapshotStatusPhase,
			common.BoolFloat64(p.value), m, p.name)
	}
}

func (s *scraper) defaultUpdate(descName string, value float64, m *dataMetric, labels ...string) {
	info := vmsnapshotMetrics[descName]
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
