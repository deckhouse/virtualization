/*
Copyright 2026 Flant JSC

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

package vmip

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/deckhouse/virtualization-controller/pkg/monitoring/metrics"
)

const MetricDeprecatedVMIPCount = "deprecated_vmip_count"

// The metric counts resources per namespace and phase instead of exposing a
// series per resource: on a fleet of thousands of vmips per-object labels would
// blow up the cardinality, and the alert takes the sum.
var vmipMetrics = map[string]metrics.MetricInfo{
	MetricDeprecatedVMIPCount: metrics.NewMetricInfo(MetricDeprecatedVMIPCount,
		"Number of deprecated VirtualMachineIPAddress resources still present in the cluster. The address of the Main network of a virtual machine is managed by the IPAddress resources of the sdn module; the virtual machines are to be moved off the VirtualMachineIPAddress resources.",
		prometheus.GaugeValue,
		[]string{"namespace", "phase"},
		nil),
}
