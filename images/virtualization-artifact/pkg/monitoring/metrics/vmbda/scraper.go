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

package vmbda

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/monitoring/metrics/promutil"
	"github.com/deckhouse/virtualization-controller/pkg/util"
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
	s.updateMetricVMBDAStatusPhase(m)
	s.updateMetricVMBDALabels(m)
	s.updateMetricVMBDAAnnotations(m)
}

func (s *scraper) updateMetricVMBDAStatusPhase(m *dataMetric) {
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

func (s *scraper) updateMetricVMBDALabels(m *dataMetric) {
	s.updateDynamic(MetricVMBDALabels, 1, m, nil, m.Labels)
}

func (s *scraper) updateMetricVMBDAAnnotations(m *dataMetric) {
	s.updateDynamic(MetricVMBDAAnnotations, 1, m, nil, m.Annotations)
}

func (s *scraper) defaultUpdate(descName string, value float64, m *dataMetric, labels ...string) {
	info := vmbdaMetrics[descName]
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

func (s *scraper) updateDynamic(name string, value float64, m *dataMetric, labelValues []string, extraLabels prometheus.Labels) {
	info := vmbdaMetrics[name]
	metric, err := promutil.NewDynamicMetric(
		info.Desc,
		info.Type,
		value,
		WithBaseLabelsByMetric(m, labelValues...),
		extraLabels,
	)
	if err != nil {
		s.log.Warn(fmt.Sprintf("Error creating the new dynamic dataMetric for %s: %s", info.Desc, err))
		return
	}
	s.ch <- metric
}
