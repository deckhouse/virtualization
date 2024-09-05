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
	s.updateVMStatusPhaseMetrics(m)
	s.updateVMCpuCoresMetrics(m)
	s.updateVMCpuCoreFractionMetrics(m)
	s.updateVMCpuRequestedCoresMetrics(m)
	s.updateVMCpuRuntimeOverheadMetrics(m)
	s.updateVMMemorySizeMetrics(m)
	s.updateVMMemoryRuntimeOverheadMetrics(m)
	s.updateVMAwaitingRestartToApplyConfigurationMetrics(m)
	s.updateVMConfigurationAppliedMetrics(m)
	s.updateVMConfigurationRunPolicyMetrics(m)
	s.updateVMPodMetrics(m)
}

func (s *scraper) updateVMStatusPhaseMetrics(m *dataMetric) {
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
			util.BoolFloat64(p.value), m, p.name)
	}
}

func (s *scraper) updateVMCpuCoresMetrics(m *dataMetric) {
	s.defaultUpdate(MetricVirtualMachineConfigurationCpuCores,
		m.CpuCores, m)
}

func (s *scraper) updateVMCpuCoreFractionMetrics(m *dataMetric) {
	s.defaultUpdate(MetricVirtualMachineConfigurationCpuCoreFraction,
		m.CpuCoreFraction, m)
}

func (s *scraper) updateVMCpuRequestedCoresMetrics(m *dataMetric) {
	s.defaultUpdate(MetricVirtualMachineConfigurationCpuRequestedCores,
		m.CpuRequestedCores, m)
}

func (s *scraper) updateVMCpuRuntimeOverheadMetrics(m *dataMetric) {
	s.defaultUpdate(MetricVirtualMachineConfigurationCpuRuntimeOverhead,
		m.CpuRuntimeOverhead, m)
}

func (s *scraper) updateVMMemorySizeMetrics(m *dataMetric) {
	s.defaultUpdate(MetricVirtualMachineConfigurationMemorySize,
		m.MemorySize, m)
}

func (s *scraper) updateVMMemoryRuntimeOverheadMetrics(m *dataMetric) {
	s.defaultUpdate(MetricVirtualMachineConfigurationMemoryRuntimeOverhead,
		m.MemoryRuntimeOverhead, m)
}

func (s *scraper) updateVMAwaitingRestartToApplyConfigurationMetrics(m *dataMetric) {
	s.defaultUpdate(MetricVirtualMachineAwaitingRestartToApplyConfiguration,
		util.BoolFloat64(m.AwaitingRestartToApplyConfiguration), m)
}

func (s *scraper) updateVMConfigurationAppliedMetrics(m *dataMetric) {
	s.defaultUpdate(MetricVirtualMachineConfigurationApplied,
		util.BoolFloat64(m.ConfigurationApplied), m)
}

func (s *scraper) updateVMConfigurationRunPolicyMetrics(m *dataMetric) {
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
			util.BoolFloat64(p.value), m, p.name)
	}
}

func (s *scraper) updateVMPodMetrics(m *dataMetric) {
	for _, p := range m.Pods {
		s.defaultUpdate(MetricVirtualMachinePod, util.BoolFloat64(p.Active), m, p.Name)
	}
}

func (s *scraper) defaultUpdate(descName string, value float64, m *dataMetric, labels ...string) {
	desc := virtualMachineMetrics[descName]
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
