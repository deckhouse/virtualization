package vmbda

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/deckhouse/virtualization-controller/pkg/monitoring/metrics"
)

const (
	MetricVMBDAStatusPhase = "virtualmachineblockdeviceattachment_status_phase"
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

var vmbdaMetrics = map[string]*prometheus.Desc{
	MetricVMBDAStatusPhase: prometheus.NewDesc(prometheus.BuildFQName(metrics.MetricNamespace, "", MetricVMBDAStatusPhase),
		"The virtualmachineblockdeviceattachment current phase.",
		WithBaseLabels("phase"),
		nil),
}
