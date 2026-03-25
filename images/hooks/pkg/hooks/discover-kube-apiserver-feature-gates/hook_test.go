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
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/testing/mock"
)

func TestDiscoverKubeAPIServerFeatureGates(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "DiscoverKubeAPIServerFeatureGates Suite")
}

func metricsLine(name, stage string, value int) string {
	return fmt.Sprintf(`kubernetes_feature_enabled{name="%s",stage="%s"} %d`, name, stage, value)
}

var _ = Describe("parseEnabledFeatureGates", func() {
	It("should return enabled feature gates only", func() {
		data := joinLines(
			metricsLine("FeatureA", "BETA", 1),
			metricsLine("FeatureB", "ALPHA", 0),
			metricsLine("FeatureC", "", 1),
			metricsLine("FeatureD", "DEPRECATED", 0),
		)

		result := parseEnabledFeatureGates([]byte(data))
		Expect(result).To(ConsistOf("FeatureA", "FeatureC"))
	})

	It("should skip comment and type lines", func() {
		data := joinLines(
			"# HELP kubernetes_feature_enabled [BETA] This metric records the data about the stage and enablement of a k8s feature.",
			"# TYPE kubernetes_feature_enabled gauge",
			metricsLine("FeatureA", "BETA", 1),
		)

		result := parseEnabledFeatureGates([]byte(data))
		Expect(result).To(ConsistOf("FeatureA"))
	})

	It("should return nil for empty input", func() {
		result := parseEnabledFeatureGates([]byte(""))
		Expect(result).To(BeNil())
	})

	It("should return nil when no feature gate metrics present", func() {
		data := joinLines(
			"# HELP apiserver_request_total",
			"# TYPE apiserver_request_total counter",
			`apiserver_request_total{verb="GET"} 42`,
		)

		result := parseEnabledFeatureGates([]byte(data))
		Expect(result).To(BeNil())
	})

	It("should return nil when all feature gates are disabled", func() {
		data := joinLines(
			metricsLine("FeatureA", "ALPHA", 0),
			metricsLine("FeatureB", "ALPHA", 0),
		)

		result := parseEnabledFeatureGates([]byte(data))
		Expect(result).To(BeNil())
	})

	It("should handle mixed metrics output", func() {
		data := joinLines(
			`apiserver_request_total{verb="GET"} 100`,
			metricsLine("DRADeviceBindingConditions", "BETA", 1),
			`apiserver_request_duration_seconds_bucket{le="0.1"} 50`,
			metricsLine("DRAConsumableCapacity", "BETA", 1),
			metricsLine("SomeDisabledFeature", "ALPHA", 0),
		)

		result := parseEnabledFeatureGates([]byte(data))
		Expect(result).To(ConsistOf("DRADeviceBindingConditions", "DRAConsumableCapacity"))
	})
})

var _ = Describe("extractLabel", func() {
	It("should extract name label", func() {
		line := `kubernetes_feature_enabled{name="FeatureA",stage="BETA"} 1`
		Expect(extractLabel(line, "name")).To(Equal("FeatureA"))
	})

	It("should extract stage label", func() {
		line := `kubernetes_feature_enabled{name="FeatureA",stage="BETA"} 1`
		Expect(extractLabel(line, "stage")).To(Equal("BETA"))
	})

	It("should return empty for missing label", func() {
		line := `kubernetes_feature_enabled{name="FeatureA"} 1`
		Expect(extractLabel(line, "stage")).To(BeEmpty())
	})

	It("should handle empty label value", func() {
		line := `kubernetes_feature_enabled{name="FeatureA",stage=""} 1`
		Expect(extractLabel(line, "stage")).To(BeEmpty())
	})
})

var _ = Describe("reconcile", func() {
	var (
		dc     *mock.DependencyContainerMock
		values *mock.OutputPatchableValuesCollectorMock
		server *httptest.Server
	)

	newInput := func() *pkg.HookInput {
		return &pkg.HookInput{
			Values: values,
			DC:     dc,
			Logger: log.NewNop(),
		}
	}

	setupServer := func(metricsBody string) {
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = fmt.Fprint(w, metricsBody)
		}))

		dc.GetClientConfigMock.Return(&rest.Config{Host: server.URL}, nil)
	}

	BeforeEach(func() {
		dc = mock.NewDependencyContainerMock(GinkgoT())
		values = mock.NewPatchableValuesCollectorMock(GinkgoT())
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	It("should set feature gates from metrics", func() {
		setupServer(joinLines(
			metricsLine("FeatureA", "BETA", 1),
			metricsLine("FeatureB", "ALPHA", 0),
			metricsLine("FeatureC", "", 1),
		))

		setValues := make(map[string]any)
		values.SetMock.Set(func(path string, v any) {
			setValues[path] = v
		})

		Expect(reconcile(context.Background(), newInput())).To(Succeed())

		Expect(setValues).To(HaveKeyWithValue(featureGatesPath, ConsistOf("FeatureA", "FeatureC")))
	})

	It("should return error when client config fails", func() {
		dc.GetClientConfigMock.Return(nil, fmt.Errorf("no kubeconfig"))

		err := reconcile(context.Background(), newInput())
		Expect(err).To(MatchError(ContainSubstring("no kubeconfig")))
	})
})

func joinLines(lines ...string) string {
	result := ""
	for i, line := range lines {
		if i > 0 {
			result += "\n"
		}
		result += line
	}
	return result
}
