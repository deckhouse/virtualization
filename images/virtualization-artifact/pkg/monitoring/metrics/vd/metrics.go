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
