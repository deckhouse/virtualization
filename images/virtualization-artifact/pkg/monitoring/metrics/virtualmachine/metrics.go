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
	"github.com/prometheus/client_golang/prometheus"

	"github.com/deckhouse/virtualization-controller/pkg/monitoring/metrics"
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

func WithBaseLabelsByMetric(m *dataMetric, labels ...string) []string {
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

	MetricVirtualMachineConfigurationRunPolicy: prometheus.NewDesc(prometheus.BuildFQName(metrics.MetricNamespace, "", MetricVirtualMachineConfigurationRunPolicy),
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
