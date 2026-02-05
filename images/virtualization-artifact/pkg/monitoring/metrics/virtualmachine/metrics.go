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
	MetricVirtualMachineStatusPhase                             = "virtualmachine_status_phase"
	MetricVirtualMachineCPUCores                                = "virtualmachine_cpu_cores"
	MetricVirtualMachineConfigurationCPUCores                   = "virtualmachine_configuration_cpu_cores"
	MetricVirtualMachineCPUCoreFraction                         = "virtualmachine_cpu_core_fraction"
	MetricVirtualMachineConfigurationCPUCoreFraction            = "virtualmachine_configuration_cpu_core_fraction"
	MetricVirtualMachineConfigurationCPURuntimeOverhead         = "virtualmachine_configuration_cpu_runtime_overhead"
	MetricVirtualMachineConfigurationMemorySizeBytes            = "virtualmachine_configuration_memory_size_bytes"
	MetricVirtualMachineConfigurationMemoryRuntimeOverheadBytes = "virtualmachine_configuration_memory_runtime_overhead_bytes"
	MetricVirtualMachineAwaitingRestartToApplyConfiguration     = "virtualmachine_awaiting_restart_to_apply_configuration"
	MetricVirtualMachineConfigurationApplied                    = "virtualmachine_configuration_applied"
	MetricVirtualMachineConfigurationRunPolicy                  = "virtualmachine_configuration_run_policy"
	MetricVirtualMachineAgentReady                              = "virtualmachine_agent_ready"
	MetricVirtualMachinePod                                     = "virtualmachine_pod"
	MetricVirtualMachineLabels                                  = "virtualmachine_labels"
	MetricVirtualMachineAnnotations                             = "virtualmachine_annotations"
	MetricVirtualMachineFirmwareUpToDate                        = "virtualmachine_firmware_up_to_date"
	MetricVirtualMachineInfo                                    = "virtualmachine_info"
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

var virtualMachineMetrics = map[string]metrics.MetricInfo{
	MetricVirtualMachineStatusPhase: metrics.NewMetricInfo(MetricVirtualMachineStatusPhase,
		"The virtualmachine current phase.",
		prometheus.GaugeValue,
		WithBaseLabels("phase"),
		nil,
	),

	MetricVirtualMachineCPUCores: metrics.NewMetricInfo(MetricVirtualMachineCPUCores,
		"The virtualmachine current core count.",
		prometheus.GaugeValue,
		WithBaseLabels(),
		nil,
	),

	MetricVirtualMachineConfigurationCPUCores: metrics.NewMetricInfo(MetricVirtualMachineConfigurationCPUCores,
		"The virtualmachine desired core count from the spec.",
		prometheus.GaugeValue,
		WithBaseLabels(),
		nil,
	),

	MetricVirtualMachineCPUCoreFraction: metrics.NewMetricInfo(MetricVirtualMachineCPUCoreFraction,
		"The virtualmachine current coreFraction.",
		prometheus.GaugeValue,
		WithBaseLabels(),
		nil,
	),

	MetricVirtualMachineConfigurationCPUCoreFraction: metrics.NewMetricInfo(MetricVirtualMachineConfigurationCPUCoreFraction,
		"The virtualmachine desired coreFraction from the spec.",
		prometheus.GaugeValue,
		WithBaseLabels(),
		nil,
	),

	MetricVirtualMachineConfigurationCPURuntimeOverhead: metrics.NewMetricInfo(MetricVirtualMachineConfigurationCPURuntimeOverhead,
		"The virtualmachine current cpu runtime overhead.",
		prometheus.GaugeValue,
		WithBaseLabels(),
		nil,
	),

	MetricVirtualMachineConfigurationMemorySizeBytes: metrics.NewMetricInfo(MetricVirtualMachineConfigurationMemorySizeBytes,
		"The virtualmachine current memory size.",
		prometheus.GaugeValue,
		WithBaseLabels(),
		nil,
	),

	MetricVirtualMachineConfigurationMemoryRuntimeOverheadBytes: metrics.NewMetricInfo(MetricVirtualMachineConfigurationMemoryRuntimeOverheadBytes,
		"The virtualmachine current memory runtime overhead.",
		prometheus.GaugeValue,
		WithBaseLabels(),
		nil,
	),

	MetricVirtualMachineAwaitingRestartToApplyConfiguration: metrics.NewMetricInfo(MetricVirtualMachineAwaitingRestartToApplyConfiguration,
		"The virtualmachine awaiting restart to apply configuration.",
		prometheus.GaugeValue,
		WithBaseLabels(),
		nil,
	),

	MetricVirtualMachineConfigurationApplied: metrics.NewMetricInfo(MetricVirtualMachineConfigurationApplied,
		"The virtualmachine configuration applied.",
		prometheus.GaugeValue,
		WithBaseLabels(),
		nil,
	),

	MetricVirtualMachineConfigurationRunPolicy: metrics.NewMetricInfo(MetricVirtualMachineConfigurationRunPolicy,
		"The virtualmachine current runPolicy.",
		prometheus.GaugeValue,
		WithBaseLabels("runPolicy"),
		nil,
	),

	MetricVirtualMachinePod: metrics.NewMetricInfo(MetricVirtualMachinePod,
		"The virtualmachine current active pod.",
		prometheus.GaugeValue,
		WithBaseLabels("pod"),
		nil,
	),

	MetricVirtualMachineLabels: metrics.NewMetricInfo(MetricVirtualMachineLabels,
		"Kubernetes labels converted to Prometheus labels.",
		prometheus.GaugeValue,
		WithBaseLabels(),
		nil,
	),

	MetricVirtualMachineAnnotations: metrics.NewMetricInfo(MetricVirtualMachineAnnotations,
		"Kubernetes annotations converted to Prometheus labels.",
		prometheus.GaugeValue,
		WithBaseLabels(),
		nil,
	),

	MetricVirtualMachineAgentReady: metrics.NewMetricInfo(MetricVirtualMachineAgentReady,
		"The virtualmachine agent ready.",
		prometheus.GaugeValue,
		WithBaseLabels(),
		nil,
	),

	MetricVirtualMachineFirmwareUpToDate: metrics.NewMetricInfo(MetricVirtualMachineFirmwareUpToDate,
		"The virtualmachine firmware up to date.",
		prometheus.GaugeValue,
		WithBaseLabels(),
		nil,
	),

	MetricVirtualMachineInfo: metrics.NewMetricInfo(MetricVirtualMachineInfo,
		"Information about the virtualmachine including the applied virtualmachineclass.",
		prometheus.GaugeValue,
		WithBaseLabels("applied_virtualmachineclass"),
		nil,
	),
}
