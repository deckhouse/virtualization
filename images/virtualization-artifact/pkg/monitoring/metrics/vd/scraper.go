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

package vd

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/monitoring/metrics/promutil"
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
	s.updateMetricDiskStatusPhase(m)
	s.updateMetricDiskLabels(m)
	s.updateMetricDiskAnnotations(m)
	s.updateMetricDiskCapacityBytes(m)
	s.updateMetricDiskInfo(m)
	s.updateMetricDiskStatusInUse(m)
}

func (s *scraper) updateMetricDiskStatusPhase(m *dataMetric) {
	phase := m.Phase
	if phase == "" {
		phase = v1alpha2.DiskPending
	}
	phases := []struct {
		value bool
		name  string
	}{
		{phase == v1alpha2.DiskPending, string(v1alpha2.DiskPending)},
		{phase == v1alpha2.DiskWaitForUserUpload, string(v1alpha2.DiskWaitForUserUpload)},
		{phase == v1alpha2.DiskWaitForFirstConsumer, string(v1alpha2.DiskWaitForFirstConsumer)},
		{phase == v1alpha2.DiskProvisioning, string(v1alpha2.DiskProvisioning)},
		{phase == v1alpha2.DiskFailed, string(v1alpha2.DiskFailed)},
		{phase == v1alpha2.DiskLost, string(v1alpha2.DiskLost)},
		{phase == v1alpha2.DiskReady, string(v1alpha2.DiskReady)},
		{phase == v1alpha2.DiskResizing, string(v1alpha2.DiskResizing)},
		{phase == v1alpha2.DiskTerminating, string(v1alpha2.DiskTerminating)},
		{phase == v1alpha2.DiskMigrating, string(v1alpha2.DiskMigrating)},
	}

	for _, p := range phases {
		s.defaultUpdate(MetricDiskStatusPhase,
			common.BoolFloat64(p.value), m, p.name)
	}
}

func (s *scraper) updateMetricDiskLabels(m *dataMetric) {
	s.updateDynamic(MetricDiskLabels, 1, m, nil, m.Labels)
}

func (s *scraper) updateMetricDiskAnnotations(m *dataMetric) {
	s.updateDynamic(MetricDiskAnnotations, 1, m, nil, m.Annotations)
}

func (s *scraper) updateMetricDiskCapacityBytes(m *dataMetric) {
	s.defaultUpdate(MetricDiskCapacityBytes, float64(m.CapacityBytes), m)
}

func (s *scraper) updateMetricDiskInfo(m *dataMetric) {
	s.defaultUpdate(MetricDiskInfo, 1, m, m.StorageClass, m.PersistentVolumeClaim)
}

func (s *scraper) updateMetricDiskStatusInUse(m *dataMetric) {
	if m.InUse && len(m.AttachedVirtualMachines) > 0 {
		for _, vmName := range m.AttachedVirtualMachines {
			s.defaultUpdate(MetricDiskStatusInUse, 1, m, vmName)
		}
	} else {
		s.defaultUpdate(MetricDiskStatusInUse, 0, m, "")
	}
}

func (s *scraper) defaultUpdate(descName string, value float64, m *dataMetric, labels ...string) {
	info := diskMetrics[descName]
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
	info := diskMetrics[name]
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
