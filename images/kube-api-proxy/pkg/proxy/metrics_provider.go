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

package proxy

import (
	"github.com/prometheus/client_golang/prometheus"

	"kube-api-proxy/pkg/monitoring/metrics"
)

const (
	proxySubsystem = "proxy"

	clientRequestsTotalName      = "client_requests_total"
	targetResponsesTotalName     = "target_responses_total"
	handledRequestsTotalName     = "handled_requests_total"
	fromClientBytesName          = "from_client_bytes_total"
	toTargetBytesName            = "to_target_bytes_total"
	fromTargetBytesName          = "from_target_bytes_total"
	toClientBytesName            = "to_client_bytes_total"
	rewritesTotalName            = "rewrites_total"
	rewritesSecondsTotalName     = "rewrites_seconds_total"
	responseInvalidJSONTotalName = "target_response_invalid_json_total"

	nameLabel     = "name"
	resourceLabel = "resource"
	methodLabel   = "method"
	decisionLabel = "decision"
	statusLabel   = "status"
	errorLabel    = "error"

	// decisionIncoming indicates rewrite for the client request.
	decisionClient  = "client"
	decisionRewrite = "rewrite"
	decisionWatch   = "watch"
	decisionPass    = "pass"

	errorOccured = "1"
	noError      = "0"
)

var (
	clientRequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Subsystem: proxySubsystem,
		Name:      clientRequestsTotalName,
		Help:      "Total number of client requests",
	}, []string{nameLabel, resourceLabel, methodLabel})
	targetResponsesTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Subsystem: proxySubsystem,
		Name:      targetResponsesTotalName,
		Help:      "Total number of responses from the target",
	}, []string{nameLabel, resourceLabel, methodLabel, statusLabel, errorLabel})
	handledRequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Subsystem: proxySubsystem,
		Name:      handledRequestsTotalName,
		Help:      "Total number of requests handled by the proxy instance",
	}, []string{nameLabel, resourceLabel, methodLabel, decisionLabel, statusLabel, errorLabel})
	fromClientBytes = prometheus.NewCounterVec(prometheus.CounterOpts{
		Subsystem: proxySubsystem,
		Name:      fromClientBytesName,
		Help:      "Total bytes received from the client",
	}, []string{nameLabel, resourceLabel, methodLabel, decisionLabel})
	toTargetBytes = prometheus.NewCounterVec(prometheus.CounterOpts{
		Subsystem: proxySubsystem,
		Name:      toTargetBytesName,
		Help:      "Total bytes transferred to the target",
	}, []string{nameLabel, resourceLabel, methodLabel, decisionLabel})
	fromTargetBytes = prometheus.NewCounterVec(prometheus.CounterOpts{
		Subsystem: proxySubsystem,
		Name:      fromTargetBytesName,
		Help:      "Total bytes received from the target",
	}, []string{nameLabel, resourceLabel, methodLabel, decisionLabel})
	toClientBytes = prometheus.NewCounterVec(prometheus.CounterOpts{
		Subsystem: proxySubsystem,
		Name:      toClientBytesName,
		Help:      "Total bytes transferred back to the client",
	}, []string{nameLabel, resourceLabel, methodLabel, decisionLabel})
	rewritesTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Subsystem: proxySubsystem,
		Name:      rewritesTotalName,
		Help:      "Total rewrites executed by the proxy instance",
	}, []string{nameLabel, resourceLabel, methodLabel, decisionLabel, errorLabel})
	rewritesSecondsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Subsystem: proxySubsystem,
		Name:      rewritesSecondsTotalName,
		Help:      "Total time spent on executing rewrites",
	}, []string{nameLabel, resourceLabel, methodLabel, decisionLabel})
	responseInvalidJSONTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Subsystem: proxySubsystem,
		Name:      responseInvalidJSONTotalName,
		Help:      "Total target responses with invalid JSON",
	}, []string{nameLabel, resourceLabel, methodLabel, decisionLabel, statusLabel})
)

func RegisterMetrics() {
	metrics.Registry.MustRegister(
		handledRequestsTotal,
		fromClientBytes,
		toTargetBytes,
		fromTargetBytes,
		toClientBytes,
		rewritesTotal,
		rewritesSecondsTotal,
	)
}

type MetricsProvider interface {
	NewClientRequestsTotal(name, resource, method string) prometheus.Counter
	NewTargetResponsesTotal(name, resource, method, status, error string) prometheus.Counter
	NewHandledRequestsTotal(name, resource, method, decision, status, error string) prometheus.Counter
	NewFromClientBytesTotal(name, resource, method, decision string) prometheus.Counter
	NewToTargetBytesTotal(name, resource, method, decision string) prometheus.Counter
	NewFromTargetBytesTotal(name, resource, method, decision string) prometheus.Counter
	NewToClientBytesTotal(name, resource, method, decision string) prometheus.Counter
	NewRewritesTotal(name, resource, method, decision, error string) prometheus.Counter
	NewRewritesSecondsTotal(name, resource, method, decision string) prometheus.Counter
	NewTargetResponseInvalidJSONTotal(name, resource, method, decision, status string) prometheus.Counter
}

func NewMetricsProvider() MetricsProvider {
	return &proxyMetricsProvider{}
}

type proxyMetricsProvider struct{}

func (p *proxyMetricsProvider) NewClientRequestsTotal(name, resource, method string) prometheus.Counter {
	return clientRequestsTotal.WithLabelValues(name, resource, method)
}

func (p *proxyMetricsProvider) NewTargetResponsesTotal(name, resource, method, status, error string) prometheus.Counter {
	return targetResponsesTotal.WithLabelValues(name, resource, method, status, error)
}

func (p *proxyMetricsProvider) NewHandledRequestsTotal(name, resource, method, decision, status, error string) prometheus.Counter {
	return handledRequestsTotal.WithLabelValues(name, resource, method, decision, status, error)
}

func (p *proxyMetricsProvider) NewFromClientBytesTotal(name, resource, method, decision string) prometheus.Counter {
	return fromClientBytes.WithLabelValues(name, resource, method, decision)
}

func (p *proxyMetricsProvider) NewToTargetBytesTotal(name, resource, method, decision string) prometheus.Counter {
	return toTargetBytes.WithLabelValues(name, resource, method, decision)
}

func (p *proxyMetricsProvider) NewFromTargetBytesTotal(name, resource, method, decision string) prometheus.Counter {
	return fromTargetBytes.WithLabelValues(name, resource, method, decision)
}

func (p *proxyMetricsProvider) NewToClientBytesTotal(name, resource, method, decision string) prometheus.Counter {
	return toClientBytes.WithLabelValues(name, resource, method, decision)
}

func (p *proxyMetricsProvider) NewRewritesTotal(name, resource, method, decision, error string) prometheus.Counter {
	return rewritesTotal.WithLabelValues(name, resource, method, decision, error)
}

func (p *proxyMetricsProvider) NewRewritesSecondsTotal(name, resource, method, decision string) prometheus.Counter {
	return rewritesSecondsTotal.WithLabelValues(name, resource, method, decision)
}

func (p *proxyMetricsProvider) NewTargetResponseInvalidJSONTotal(name, resource, method, decision, status string) prometheus.Counter {
	return responseInvalidJSONTotal.WithLabelValues(name, resource, method, decision, status)
}

func NoopMetricsProvider() MetricsProvider {
	return noopMetricsProvider{}
}

type noopMetric struct {
	prometheus.Metric
	prometheus.Collector
}

func (noopMetric) Inc()            {}
func (noopMetric) Dec()            {}
func (noopMetric) Set(float64)     {}
func (noopMetric) Add(float64)     {}
func (noopMetric) Observe(float64) {}

type noopMetricsProvider struct{}

func (_ noopMetricsProvider) NewClientRequestsTotal(name, resource, method string) prometheus.Counter {
	return noopMetric{}
}
func (_ noopMetricsProvider) NewTargetResponsesTotal(name, resource, method, status, error string) prometheus.Counter {
	return noopMetric{}
}
func (_ noopMetricsProvider) NewHandledRequestsTotal(name, resource, method, decision, status, error string) prometheus.Counter {
	return noopMetric{}
}
func (_ noopMetricsProvider) NewFromClientBytesTotal(name, resource, method, decision string) prometheus.Counter {
	return noopMetric{}
}
func (_ noopMetricsProvider) NewToTargetBytesTotal(name, resource, method, decision string) prometheus.Counter {
	return noopMetric{}
}
func (_ noopMetricsProvider) NewFromTargetBytesTotal(name, resource, method, decision string) prometheus.Counter {
	return noopMetric{}
}
func (_ noopMetricsProvider) NewToClientBytesTotal(name, resource, method, decision string) prometheus.Counter {
	return noopMetric{}
}
func (_ noopMetricsProvider) NewRewritesTotal(_, _, _, _, _ string) prometheus.Counter {
	return noopMetric{}
}
func (_ noopMetricsProvider) NewRewritesSecondsTotal(_, _, _, _ string) prometheus.Counter {
	return noopMetric{}
}
func (_ noopMetricsProvider) NewTargetResponseInvalidJSONTotal(name, resource, method, decision, status string) prometheus.Counter {
	return noopMetric{}
}
