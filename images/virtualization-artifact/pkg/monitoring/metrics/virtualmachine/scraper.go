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

package virtualmachine

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
	s.updateMetricVirtualMachineStatusPhase(m)
	s.updateMetricVirtualMachineCPUCores(m)
	s.updateMetricVirtualMachineConfigurationCPUCores(m)
	s.updateMetricVirtualMachineCPUCoreFraction(m)
	s.updateMetricVirtualMachineConfigurationCPUCoreFraction(m)
	s.updateMetricVirtualMachineConfigurationCPURuntimeOverhead(m)
	s.updateMetricVirtualMachineConfigurationMemoryRuntimeOverheadBytes(m)
	s.updateMetricVirtualMachineConfigurationMemorySizeBytes(m)
	s.updateMetricVirtualMachineAwaitingRestartToApplyConfiguration(m)
	s.updateMetricVirtualMachineConfigurationApplied(m)
	s.updateMetricVirtualMachineConfigurationRunPolicy(m)
	s.updateMetricVirtualMachinePod(m)
	s.updateMetricVirtualMachineLabels(m)
	s.updateMetricVirtualMachineAnnotations(m)
	s.updateMetricVirtualMachineAgentReady(m)
	s.updateMetricVirtualMachineFirmwareUpToDate(m)
	s.updateMetricVirtualMachineInfo(m)
}

func (s *scraper) updateMetricVirtualMachineStatusPhase(m *dataMetric) {
	phase := m.Phase
	if phase == "" {
		phase = v1alpha2.MachinePending
	}
	phases := []struct {
		value bool
		name  string
	}{
		{phase == v1alpha2.MachinePending, string(v1alpha2.MachinePending)},
		{phase == v1alpha2.MachineRunning, string(v1alpha2.MachineRunning)},
		{phase == v1alpha2.MachineDegraded, string(v1alpha2.MachineDegraded)},
		{phase == v1alpha2.MachineTerminating, string(v1alpha2.MachineTerminating)},
		{phase == v1alpha2.MachineStopped, string(v1alpha2.MachineStopped)},
		{phase == v1alpha2.MachineStopping, string(v1alpha2.MachineStopping)},
		{phase == v1alpha2.MachineStarting, string(v1alpha2.MachineStarting)},
		{phase == v1alpha2.MachineMigrating, string(v1alpha2.MachineMigrating)},
		{phase == v1alpha2.MachinePause, string(v1alpha2.MachinePause)},
	}
	for _, p := range phases {
		s.defaultUpdate(MetricVirtualMachineStatusPhase,
			common.BoolFloat64(p.value), m, p.name)
	}
}

func (s *scraper) updateMetricVirtualMachineCPUCores(m *dataMetric) {
	s.defaultUpdate(MetricVirtualMachineCPUCores,
		m.CPUCores, m)
}

func (s *scraper) updateMetricVirtualMachineConfigurationCPUCores(m *dataMetric) {
	s.defaultUpdate(MetricVirtualMachineConfigurationCPUCores,
		m.CPUConfigurationCores, m)
}

func (s *scraper) updateMetricVirtualMachineCPUCoreFraction(m *dataMetric) {
	s.defaultUpdate(MetricVirtualMachineCPUCoreFraction,
		m.CPUCoreFraction, m)
}

func (s *scraper) updateMetricVirtualMachineConfigurationCPUCoreFraction(m *dataMetric) {
	s.defaultUpdate(MetricVirtualMachineConfigurationCPUCoreFraction,
		m.CPUConfigurationCoreFraction, m)
}

func (s *scraper) updateMetricVirtualMachineConfigurationCPURuntimeOverhead(m *dataMetric) {
	s.defaultUpdate(MetricVirtualMachineConfigurationCPURuntimeOverhead,
		m.CPURuntimeOverhead, m)
}

func (s *scraper) updateMetricVirtualMachineConfigurationMemoryRuntimeOverheadBytes(m *dataMetric) {
	s.defaultUpdate(MetricVirtualMachineConfigurationMemoryRuntimeOverheadBytes,
		m.MemoryRuntimeOverhead, m)
}

func (s *scraper) updateMetricVirtualMachineConfigurationMemorySizeBytes(m *dataMetric) {
	s.defaultUpdate(MetricVirtualMachineConfigurationMemorySizeBytes,
		m.MemoryConfigurationSize, m)
}

func (s *scraper) updateMetricVirtualMachineAwaitingRestartToApplyConfiguration(m *dataMetric) {
	s.defaultUpdate(MetricVirtualMachineAwaitingRestartToApplyConfiguration,
		common.BoolFloat64(m.AwaitingRestartToApplyConfiguration), m)
}

func (s *scraper) updateMetricVirtualMachineConfigurationApplied(m *dataMetric) {
	s.defaultUpdate(MetricVirtualMachineConfigurationApplied,
		common.BoolFloat64(m.ConfigurationApplied), m)
}

func (s *scraper) updateMetricVirtualMachineAgentReady(m *dataMetric) {
	s.defaultUpdate(MetricVirtualMachineAgentReady, common.BoolFloat64(m.AgentReady), m)
}

func (s *scraper) updateMetricVirtualMachineConfigurationRunPolicy(m *dataMetric) {
	policy := m.RunPolicy
	policies := []struct {
		value bool
		name  string
	}{
		{policy == v1alpha2.AlwaysOnPolicy, string(v1alpha2.AlwaysOnPolicy)},
		{policy == v1alpha2.AlwaysOffPolicy, string(v1alpha2.AlwaysOffPolicy)},
		{policy == v1alpha2.ManualPolicy, string(v1alpha2.ManualPolicy)},
		{policy == v1alpha2.AlwaysOnUnlessStoppedManually, string(v1alpha2.AlwaysOnUnlessStoppedManually)},
	}
	for _, p := range policies {
		s.defaultUpdate(MetricVirtualMachineConfigurationRunPolicy,
			common.BoolFloat64(p.value), m, p.name)
	}
}

func (s *scraper) updateMetricVirtualMachinePod(m *dataMetric) {
	for _, p := range m.Pods {
		s.defaultUpdate(MetricVirtualMachinePod, common.BoolFloat64(p.Active), m, p.Name)
	}
}

func (s *scraper) updateMetricVirtualMachineLabels(m *dataMetric) {
	s.updateDynamic(MetricVirtualMachineLabels, 1, m, nil, m.Labels)
}

func (s *scraper) updateMetricVirtualMachineAnnotations(m *dataMetric) {
	s.updateDynamic(MetricVirtualMachineAnnotations, 1, m, nil, m.Annotations)
}

func (s *scraper) updateMetricVirtualMachineFirmwareUpToDate(m *dataMetric) {
	s.defaultUpdate(MetricVirtualMachineFirmwareUpToDate, common.BoolFloat64(m.firmwareUpToDate), m)
}

func (s *scraper) updateMetricVirtualMachineInfo(m *dataMetric) {
	s.defaultUpdate(MetricVirtualMachineInfo, 1, m, m.AppliedVirtualMachineClassName)
}

func (s *scraper) defaultUpdate(name string, value float64, m *dataMetric, labelValues ...string) {
	info := virtualMachineMetrics[name]
	metric, err := prometheus.NewConstMetric(
		info.Desc,
		info.Type,
		value,
		WithBaseLabelsByMetric(m, labelValues...)...,
	)
	if err != nil {
		s.log.Warn(fmt.Sprintf("Error creating the new const dataMetric for %s: %s", info.Desc, err))
		return
	}
	s.ch <- metric
}

func (s *scraper) updateDynamic(name string, value float64, m *dataMetric, labelValues []string, extraLabels prometheus.Labels) {
	info := virtualMachineMetrics[name]
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
