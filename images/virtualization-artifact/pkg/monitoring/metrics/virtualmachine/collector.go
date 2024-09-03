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
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/monitoring/metrics"
	"github.com/deckhouse/virtualization-controller/pkg/util"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	MetricVirtualMachineStatusPhase                         = "virtualmachine_status_phase"
	MetricVirtualMachineConfigurationCpuCores               = "virtualmachine_configuration_cpu_cores"
	MetricVirtualMachineConfigurationCpuCoreFraction        = "virtualmachine_configuration_cpu_core_fraction"
	MetricVirtualMachineConfigurationCpuRequestedCores      = "virtualmachine_configuration_cpu_requested_cores"
	MetricVirtualMachineConfigurationCpuRuntimeOverhead     = "virtualmachine_configuration_cpu_runtime_overhead"
	MetricVirtualMachineConfigurationMemorySize             = "virtualmachine_configuration_memory_size"
	MetricVirtualMachineConfigurationMemoryRuntimeOverhead  = "virtualmachine_configuration_memory_runtime_overhead"
	MetricVirtualMachineAwaitingRestartToApplyConfiguration = "virtualmachine_awaiting_restart_to_apply_configuration"
	MetricVirtualMachineConfigurationApplied                = "virtualmachine_configuration_applied"
	MetricVirtualMachineConfigurationRunPolicy              = "virtualmachine_configuration_run_policy"
	MetricVirtualMachinePod                                 = "virtualmachine_pod"
)

var baseLabels = []string{"name", "namespace", "uid", "node"}

func WithBaseLabels(labels ...string) []string {
	return append(baseLabels, labels...)
}

func WithBaseLabelsByMetric(m *metric, labels ...string) []string {
	var base []string
	if m != nil {
		base = []string{
			m.Name,
			m.Namespace,
			m.UID,
			m.Node,
		}
	}
	return append(base, labels...)
}

var virtualMachineMetrics = map[string]*prometheus.Desc{
	MetricVirtualMachineStatusPhase: prometheus.NewDesc(prometheus.BuildFQName(metrics.MetricNamespace, "", MetricVirtualMachineStatusPhase),
		"The virtualmachine current phase.",
		WithBaseLabels("phase"),
		nil,
	),

	MetricVirtualMachineConfigurationCpuCores: prometheus.NewDesc(prometheus.BuildFQName(metrics.MetricNamespace, "", MetricVirtualMachineConfigurationCpuCores),
		"The virtualmachine current core count.",
		WithBaseLabels(),
		nil,
	),

	MetricVirtualMachineConfigurationCpuCoreFraction: prometheus.NewDesc(prometheus.BuildFQName(metrics.MetricNamespace, "", MetricVirtualMachineConfigurationCpuCoreFraction),
		"The virtualmachine current coreFraction.",
		WithBaseLabels(),
		nil,
	),

	MetricVirtualMachineConfigurationCpuRequestedCores: prometheus.NewDesc(prometheus.BuildFQName(metrics.MetricNamespace, "", MetricVirtualMachineConfigurationCpuRequestedCores),
		"The virtualmachine current requested cores.",
		WithBaseLabels(),
		nil,
	),

	MetricVirtualMachineConfigurationCpuRuntimeOverhead: prometheus.NewDesc(prometheus.BuildFQName(metrics.MetricNamespace, "", MetricVirtualMachineConfigurationCpuRuntimeOverhead),
		"The virtualmachine current cpu runtime overhead.",
		WithBaseLabels(),
		nil,
	),

	MetricVirtualMachineConfigurationMemorySize: prometheus.NewDesc(prometheus.BuildFQName(metrics.MetricNamespace, "", MetricVirtualMachineConfigurationMemorySize),
		"The virtualmachine current memory size.",
		WithBaseLabels(),
		nil,
	),

	MetricVirtualMachineConfigurationMemoryRuntimeOverhead: prometheus.NewDesc(prometheus.BuildFQName(metrics.MetricNamespace, "", MetricVirtualMachineConfigurationMemoryRuntimeOverhead),
		"The virtualmachine current memory runtime overhead.",
		WithBaseLabels(),
		nil,
	),

	MetricVirtualMachineAwaitingRestartToApplyConfiguration: prometheus.NewDesc(prometheus.BuildFQName(metrics.MetricNamespace, "", MetricVirtualMachineAwaitingRestartToApplyConfiguration),
		"The virtualmachine awaiting restart to apply configuration.",
		WithBaseLabels(),
		nil,
	),

	MetricVirtualMachineConfigurationApplied: prometheus.NewDesc(prometheus.BuildFQName(metrics.MetricNamespace, "", MetricVirtualMachineConfigurationApplied),
		"The virtualmachine configuration applied.",
		WithBaseLabels(),
		nil,
	),

	MetricVirtualMachineConfigurationRunPolicy: prometheus.NewDesc(prometheus.BuildFQName(metrics.MetricNamespace, "", MetricVirtualMachineConfigurationApplied),
		"The virtualmachine current runPolicy.",
		WithBaseLabels("runPolicy"),
		nil,
	),
	MetricVirtualMachinePod: prometheus.NewDesc(prometheus.BuildFQName(metrics.MetricNamespace, "", MetricVirtualMachinePod),
		"The virtualmachine current active pod.",
		WithBaseLabels("pod"),
		nil,
	),
}

func SetupCollector(reader client.Reader, registerer prometheus.Registerer) *Collector {
	c := &Collector{
		iterator: newUnsafeIterator(reader),
	}

	registerer.MustRegister(c)
	return c
}

type handler func(m *metric) (stop bool)

type Iterator interface {
	Iter(ctx context.Context, h handler) error
}

type Collector struct {
	iterator Iterator
}

func (c Collector) Describe(ch chan<- *prometheus.Desc) {
	for _, v := range virtualMachineMetrics {
		ch <- v
	}
}

func (c Collector) Collect(ch chan<- prometheus.Metric) {
	s := newScraper(ch)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := c.iterator.Iter(ctx, func(m *metric) (stop bool) {
		s.Report(m)
		return
	}); err != nil {
		klog.Errorf("Failed to itereate of VirtualMachines: %v", err)
		return
	}
}

func newScraper(ch chan<- prometheus.Metric) *scraper {
	return &scraper{ch: ch}
}

type scraper struct {
	ch chan<- prometheus.Metric
}

func (s *scraper) Report(m *metric) {
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

func (s *scraper) updateVMStatusPhaseMetrics(m *metric) {
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

func (s *scraper) updateVMCpuCoresMetrics(m *metric) {
	s.defaultUpdate(MetricVirtualMachineConfigurationCpuCores,
		m.CpuCores, m)
}

func (s *scraper) updateVMCpuCoreFractionMetrics(m *metric) {
	s.defaultUpdate(MetricVirtualMachineConfigurationCpuCoreFraction,
		m.CpuCoreFraction, m)
}

func (s *scraper) updateVMCpuRequestedCoresMetrics(m *metric) {
	s.defaultUpdate(MetricVirtualMachineConfigurationCpuRequestedCores,
		m.CpuRequestedCores, m)
}

func (s *scraper) updateVMCpuRuntimeOverheadMetrics(m *metric) {
	s.defaultUpdate(MetricVirtualMachineConfigurationCpuRuntimeOverhead,
		m.CpuRuntimeOverhead, m)
}

func (s *scraper) updateVMMemorySizeMetrics(m *metric) {
	s.defaultUpdate(MetricVirtualMachineConfigurationMemorySize,
		m.MemorySize, m)
}

func (s *scraper) updateVMMemoryRuntimeOverheadMetrics(m *metric) {
	s.defaultUpdate(MetricVirtualMachineConfigurationMemoryRuntimeOverhead,
		m.MemoryRuntimeOverhead, m)
}

func (s *scraper) updateVMAwaitingRestartToApplyConfigurationMetrics(m *metric) {
	s.defaultUpdate(MetricVirtualMachineAwaitingRestartToApplyConfiguration,
		util.BoolFloat64(m.AwaitingRestartToApplyConfiguration), m)
}

func (s *scraper) updateVMConfigurationAppliedMetrics(m *metric) {
	s.defaultUpdate(MetricVirtualMachineConfigurationApplied,
		util.BoolFloat64(m.ConfigurationApplied), m)
}

func (s *scraper) updateVMConfigurationRunPolicyMetrics(m *metric) {
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

func (s *scraper) updateVMPodMetrics(m *metric) {
	for _, p := range m.Pods {
		s.defaultUpdate(MetricVirtualMachinePod, util.BoolFloat64(p.Active), m, p.Name)
	}
}

func (s *scraper) defaultUpdate(descName string, value float64, m *metric, labels ...string) {
	desc := virtualMachineMetrics[descName]
	metric, err := prometheus.NewConstMetric(
		desc,
		prometheus.GaugeValue,
		value,
		WithBaseLabelsByMetric(m, labels...)...,
	)
	if err != nil {
		klog.Warningf("Error creating the new const metric for %s: %s", desc, err)
		return
	}
	s.ch <- metric
}
