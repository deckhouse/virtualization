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
	"context"
	"strconv"

	"kube-api-proxy/pkg/labels"
)

type ProxyMetrics struct {
	provider MetricsProvider
	name     string
	resource string
	method   string
	decision string
	status   string
}

func NewProxyMetrics(ctx context.Context, provider MetricsProvider) *ProxyMetrics {
	return &ProxyMetrics{
		provider: provider,
		name:     labels.NameFromContext(ctx),
		resource: labels.ResourceFromContext(ctx),
		method:   labels.MethodFromContext(ctx),
		decision: labels.DecisionFromContext(ctx),
		status:   labels.StatusFromContext(ctx),
	}
}

func (p *ProxyMetrics) GotClientRequest() {
	p.provider.NewClientRequestsTotal(p.name, p.resource, p.method).Inc()
}

func (p *ProxyMetrics) TargetResponseSuccess() {
	p.provider.NewTargetResponsesTotal(p.name, p.resource, p.method, p.status, noError).Inc()
}

func (p *ProxyMetrics) TargetResponseError() {
	p.provider.NewTargetResponsesTotal(p.name, p.resource, p.method, "0", errorOccured).Inc()
}

func (p *ProxyMetrics) RewriteError() {
	p.provider.NewRewritesTotal(p.name, p.resource, p.method, p.decision, errorOccured).Inc()
}

func (p *ProxyMetrics) RewriteSuccess() {
	p.provider.NewRewritesTotal(p.name, p.resource, p.method, p.decision, noError).Inc()
}

func (p *ProxyMetrics) ClientRewriteError() {
	p.provider.NewRewritesTotal(p.name, p.resource, p.method, decisionClient, errorOccured).Inc()
}

func (p *ProxyMetrics) ClientRewriteSuccess() {
	p.provider.NewRewritesTotal(p.name, p.resource, p.method, decisionClient, noError).Inc()
}

func (p *ProxyMetrics) HandledRequestsSuccess() {
	p.provider.NewHandledRequestsTotal(p.name, p.resource, p.method, p.decision, p.status, noError).Inc()
}
func (p *ProxyMetrics) HandledRequestsError() {
	p.provider.NewHandledRequestsTotal(p.name, p.resource, p.method, p.decision, p.status, errorOccured).Inc()
}

func (p *ProxyMetrics) TargetResponseInvalidJSON(status int) {
	p.provider.NewTargetResponseInvalidJSONTotal(p.name, p.resource, p.method, p.decision, strconv.Itoa(status))
}

func (p *ProxyMetrics) FromClientBytesAdd(count int) {
	p.provider.NewFromClientBytesTotal(p.name, p.resource, p.method, decisionClient).Add(float64(count))
}

func (p *ProxyMetrics) ToTargetBytesAdd(count int) {
	p.provider.NewToTargetBytesTotal(p.name, p.resource, p.method, decisionClient).Add(float64(count))
}

func (p *ProxyMetrics) FromTargetBytesAdd(count int) {
	p.provider.NewFromTargetBytesTotal(p.name, p.resource, p.method, p.decision).Add(float64(count))
}

func (p *ProxyMetrics) ToClientBytesAdd(count int) {
	p.provider.NewToClientBytesTotal(p.name, p.resource, p.method, p.decision).Add(float64(count))
}
