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

package discover_kube_apiserver_feature_gates

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/client-go/kubernetes"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/registry"
	"github.com/deckhouse/virtualization/hooks/pkg/settings"
)

const (
	featureGatesPath = "virtualization.internal.kubeAPIServerFeatureGates"

	metricPrefix = "kubernetes_feature_enabled{"
)

var _ = registry.RegisterFunc(config, reconcile)

var config = &pkg.HookConfig{
	OnBeforeHelm: &pkg.OrderedConfig{Order: 5},
	Queue:        fmt.Sprintf("modules/%s", settings.ModuleName),
}

func reconcile(ctx context.Context, input *pkg.HookInput) error {
	metricsData, err := fetchMetrics(ctx, input.DC)
	if err != nil {
		return fmt.Errorf("failed to fetch kube-apiserver metrics: %w", err)
	}

	featureGates := parseEnabledFeatureGates(metricsData)

	input.Values.Set(featureGatesPath, featureGates)

	return nil
}

func fetchMetrics(ctx context.Context, dc pkg.DependencyContainer) ([]byte, error) {
	cfg, err := dc.GetClientConfig()
	if err != nil {
		return nil, fmt.Errorf("get client config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("create kubernetes clientset: %w", err)
	}

	result := clientset.RESTClient().Get().AbsPath("/metrics").Do(ctx)
	if err := result.Error(); err != nil {
		return nil, fmt.Errorf("request /metrics: %w", err)
	}

	raw, err := result.Raw()
	if err != nil {
		return nil, fmt.Errorf("read metrics response: %w", err)
	}

	return raw, nil
}

// parseEnabledFeatureGates extracts feature gate names from Prometheus metrics
// where kubernetes_feature_enabled gauge value equals 1.
func parseEnabledFeatureGates(data []byte) []string {
	var enabled []string

	for line := range strings.SplitSeq(string(data), "\n") {
		if !strings.HasPrefix(line, metricPrefix) {
			continue
		}

		if !strings.HasSuffix(strings.TrimSpace(line), " 1") {
			continue
		}

		name := extractLabel(line, "name")
		if name != "" {
			enabled = append(enabled, name)
		}
	}

	return enabled
}

func extractLabel(metric, label string) string {
	key := label + `="`
	idx := strings.Index(metric, key)
	if idx < 0 {
		return ""
	}

	start := idx + len(key)
	end := strings.Index(metric[start:], `"`)
	if end < 0 {
		return ""
	}

	return metric[start : start+end]
}
