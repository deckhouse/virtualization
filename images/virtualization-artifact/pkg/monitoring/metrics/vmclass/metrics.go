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

package vmclass

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/deckhouse/virtualization-controller/pkg/monitoring/metrics"
)

const (
	MetricVMClassStatusPhase   = "virtualmachineclass_status_phase"
	MetricVMClassInfo          = "virtualmachineclass_info"
	MetricVMClassAvailableNode = "virtualmachineclass_available_node"
)

var baseLabels = []string{"name", "uid"}

func WithBaseLabels(labels ...string) []string {
	return append(baseLabels, labels...)
}

func WithBaseLabelsByMetric(m *dataMetric, labels ...string) []string {
	var base []string
	if m != nil {
		base = []string{
			m.Name,
			m.UID,
		}
	}
	return append(base, labels...)
}

var vmclassMetrics = map[string]metrics.MetricInfo{
	MetricVMClassStatusPhase: metrics.NewMetricInfo(MetricVMClassStatusPhase,
		"The virtualmachineclass current phase.",
		prometheus.GaugeValue,
		WithBaseLabels("phase"),
		nil),
	MetricVMClassInfo: metrics.NewMetricInfo(MetricVMClassInfo,
		"The virtualmachineclass details: the requested CPU type.",
		prometheus.GaugeValue,
		WithBaseLabels("type"),
		nil),
	// One series per (class, node) pair instead of a plain count: the number of nodes is a
	// `count by (name)` away, while the list of nodes cannot be recovered from a number.
	// The `node` label is also what free capacity per class is derived from, by joining this
	// metric with the allocatable and requested resources of the node.
	MetricVMClassAvailableNode: metrics.NewMetricInfo(MetricVMClassAvailableNode,
		"The node that supports the CPU model of this virtualmachineclass. One series per node.",
		prometheus.GaugeValue,
		WithBaseLabels("node"),
		nil),
}
