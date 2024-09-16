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
	MetricDiskStatusPhase = "virtualdisk_status_phase"
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

var diskMetrics = map[string]*prometheus.Desc{
	MetricDiskStatusPhase: prometheus.NewDesc(prometheus.BuildFQName(metrics.MetricNamespace, "", MetricDiskStatusPhase),
		"The virtualdisk current phase.",
		WithBaseLabels("phase"),
		nil),
}