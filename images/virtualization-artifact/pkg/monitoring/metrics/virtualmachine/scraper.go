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
	s.updateMetricVirtualMachineStatusPhase(m)
	s.updateMetricVirtualMachineCpuCores(m)
	s.updateMetricVirtualMachineConfigurationCpuCores(m)
	s.updateMetricVirtualMachineCpuCoreFraction(m)
	s.updateMetricVirtualMachineConfigurationCpuCoreFraction(m)
	s.updateMetricVirtualMachineConfigurationCpuRuntimeOverhead(m)
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
}

func (s *scraper) updateMetricVirtualMachineStatusPhase(m *dataMetric) {
	phase := m.Phase
	if phase == "" {
		phase = virtv2.MachinePending
	}
	phases := []struct {
		value bool
		name  string
	}{
		{phase == virtv2.MachinePending, string(virtv2.MachinePending)},
		{phase == virtv2.MachineRunning, string(virtv2.MachineRunning)},
		{phase == virtv2.MachineDegraded, string(virtv2.MachineDegraded)},
		{phase == virtv2.MachineTerminating, string(virtv2.MachineTerminating)},
		{phase == virtv2.MachineStopped, string(virtv2.MachineStopped)},
		{phase == virtv2.MachineStopping, string(virtv2.MachineStopping)},
		{phase == virtv2.MachineStarting, string(virtv2.MachineStarting)},
		{phase == virtv2.MachineMigrating, string(virtv2.MachineMigrating)},
		{phase == virtv2.MachinePause, string(virtv2.MachinePause)},
	}
	for _, p := range phases {
		s.defaultUpdate(MetricVirtualMachineStatusPhase,
			common.BoolFloat64(p.value), m, p.name)
	}
}

func (s *scraper) updateMetricVirtualMachineCpuCores(m *dataMetric) {
	s.defaultUpdate(MetricVirtualMachineCpuCores,
		m.CpuCores, m)
}

func (s *scraper) updateMetricVirtualMachineConfigurationCpuCores(m *dataMetric) {
	s.defaultUpdate(MetricVirtualMachineConfigurationCpuCores,
		m.CpuConfigurationCores, m)
}

func (s *scraper) updateMetricVirtualMachineCpuCoreFraction(m *dataMetric) {
	s.defaultUpdate(MetricVirtualMachineCpuCoreFraction,
		m.CpuCoreFraction, m)
}

func (s *scraper) updateMetricVirtualMachineConfigurationCpuCoreFraction(m *dataMetric) {
	s.defaultUpdate(MetricVirtualMachineConfigurationCpuCoreFraction,
		m.CpuConfigurationCoreFraction, m)
}

func (s *scraper) updateMetricVirtualMachineConfigurationCpuRuntimeOverhead(m *dataMetric) {
	s.defaultUpdate(MetricVirtualMachineConfigurationCpuRuntimeOverhead,
		m.CpuRuntimeOverhead, m)
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
		{policy == virtv2.AlwaysOnPolicy, string(virtv2.AlwaysOnPolicy)},
		{policy == virtv2.AlwaysOffPolicy, string(virtv2.AlwaysOffPolicy)},
		{policy == virtv2.ManualPolicy, string(virtv2.ManualPolicy)},
		{policy == virtv2.AlwaysOnUnlessStoppedManually, string(virtv2.AlwaysOnUnlessStoppedManually)},
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
