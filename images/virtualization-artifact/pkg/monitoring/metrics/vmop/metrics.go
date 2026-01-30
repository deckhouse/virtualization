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

package vmop

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/deckhouse/virtualization-controller/pkg/monitoring/metrics"
)

const (
	MetricVMOPStatusPhase       = "virtualmachineoperation_status_phase"
	MetricVMOPCreatedTimestamp  = "virtualmachineoperation_created_timestamp"
	MetricVMOPStartedTimestamp  = "virtualmachineoperation_started_timestamp"
	MetricVMOPFinishedTimestamp = "virtualmachineoperation_finished_timestamp"
)

var baseLabels = []string{"name", "namespace", "uid", "type", "virtualmachine"}

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
			m.Type,
			m.VirtualMachine,
		}
	}
	return append(base, labels...)
}

var vmopMetrics = map[string]metrics.MetricInfo{
	MetricVMOPStatusPhase: metrics.NewMetricInfo(MetricVMOPStatusPhase,
		"The virtualmachineoperation current phase.",
		prometheus.GaugeValue,
		WithBaseLabels("phase"),
		nil),
	MetricVMOPCreatedTimestamp: metrics.NewMetricInfo(MetricVMOPCreatedTimestamp,
		"The timestamp when virtualmachineoperation was created (Unix timestamp).",
		prometheus.GaugeValue,
		WithBaseLabels(),
		nil),
	MetricVMOPStartedTimestamp: metrics.NewMetricInfo(MetricVMOPStartedTimestamp,
		"The timestamp when virtualmachineoperation transitioned to InProgress phase (Unix timestamp).",
		prometheus.GaugeValue,
		WithBaseLabels(),
		nil),
	MetricVMOPFinishedTimestamp: metrics.NewMetricInfo(MetricVMOPFinishedTimestamp,
		"The timestamp when virtualmachineoperation finished (Completed or Failed phase, Unix timestamp).",
		prometheus.GaugeValue,
		WithBaseLabels(),
		nil),
}
