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
	"github.com/prometheus/client_golang/prometheus"

	"github.com/deckhouse/virtualization-controller/pkg/monitoring/metrics"
)

const (
	MetricDiskStatusPhase   = "virtualdisk_status_phase"
	MetricDiskLabels        = "virtualdisk_labels"
	MetricDiskAnnotations   = "virtualdisk_annotations"
	MetricDiskCapacityBytes = "virtualdisk_capacity_bytes"
	MetricDiskInfo          = "virtualdisk_info"
	MetricDiskStatusInUse   = "virtualdisk_status_in_use"
)

var baseLabels = []string{"name", "namespace", "uid"}

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
		}
	}
	return append(base, labels...)
}

var diskMetrics = map[string]metrics.MetricInfo{
	MetricDiskStatusPhase: metrics.NewMetricInfo(
		MetricDiskStatusPhase,
		"The virtualdisk current phase.",
		prometheus.GaugeValue,
		WithBaseLabels("phase"),
		nil,
	),

	MetricDiskLabels: metrics.NewMetricInfo(MetricDiskLabels,
		"Kubernetes labels converted to Prometheus labels.",
		prometheus.GaugeValue,
		WithBaseLabels(),
		nil,
	),

	MetricDiskAnnotations: metrics.NewMetricInfo(MetricDiskAnnotations,
		"Kubernetes annotations converted to Prometheus labels.",
		prometheus.GaugeValue,
		WithBaseLabels(),
		nil,
	),

	MetricDiskCapacityBytes: metrics.NewMetricInfo(
		MetricDiskCapacityBytes,
		"The virtualdisk capacity in bytes.",
		prometheus.GaugeValue,
		baseLabels,
		nil,
	),

	MetricDiskInfo: metrics.NewMetricInfo(
		MetricDiskInfo,
		"Information about the virtualdisk.",
		prometheus.GaugeValue,
		WithBaseLabels("storageclass", "persistentvolumeclaim"),
		nil,
	),

	MetricDiskStatusInUse: metrics.NewMetricInfo(
		MetricDiskStatusInUse,
		"Whether the virtualdisk is in use (1 - yes, 0 - no).",
		prometheus.GaugeValue,
		WithBaseLabels("virtualmachine"),
		nil,
	),
}
