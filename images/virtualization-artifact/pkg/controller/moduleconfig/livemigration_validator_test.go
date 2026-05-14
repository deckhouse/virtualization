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

package moduleconfig

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	mcapi "github.com/deckhouse/virtualization-controller/pkg/controller/moduleconfig/api"
)

var _ = Describe("parseLiveMigrationSystemNetworkName", func() {
	DescribeTable("extracts the configured name",
		func(settings mcapi.SettingsValues, expected string) {
			Expect(parseLiveMigrationSystemNetworkName(settings)).To(Equal(expected))
		},
		Entry("nil settings", mcapi.SettingsValues(nil), ""),
		Entry("missing liveMigration", mcapi.SettingsValues{}, ""),
		Entry("liveMigration without systemNetworkName",
			mcapi.SettingsValues{"liveMigration": map[string]any{}}, ""),
		Entry("happy path",
			mcapi.SettingsValues{"liveMigration": map[string]any{"systemNetworkName": "migration"}},
			"migration"),
		Entry("non-string value treated as absent",
			mcapi.SettingsValues{"liveMigration": map[string]any{"systemNetworkName": 42}}, ""),
	)
})

var _ = Describe("liveMigrationValidator", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	mcWith := func(name string) *mcapi.ModuleConfig {
		mc := &mcapi.ModuleConfig{}
		mc.Name = moduleConfigName
		if name != "" {
			mc.Spec.Settings = mcapi.SettingsValues{
				"liveMigration": map[string]any{"systemNetworkName": name},
			}
		}
		return mc
	}

	systemNetwork := func(name string, ready bool) *unstructured.Unstructured {
		sn := &unstructured.Unstructured{}
		sn.SetGroupVersionKind(systemNetworkGVK)
		sn.SetName(name)
		status := "False"
		if ready {
			status = "True"
		}
		_ = unstructured.SetNestedSlice(sn.Object, []any{
			map[string]any{
				"type":   conditionTypeReady,
				"status": status,
			},
		}, "status", "conditions")
		return sn
	}

	It("accepts MC without liveMigration block", func() {
		v := liveMigrationValidator{client: fake.NewClientBuilder().Build()}
		warns, err := v.validate(ctx, mcWith(""))
		Expect(err).NotTo(HaveOccurred())
		Expect(warns).To(BeNil())
	})

	It("rejects when SystemNetwork is not found", func() {
		scheme := runtime.NewScheme()
		v := liveMigrationValidator{client: fake.NewClientBuilder().WithScheme(scheme).Build()}
		_, err := v.validate(ctx, mcWith("missing"))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("SystemNetwork not found"))
	})

	It("accepts when SystemNetwork is Ready", func() {
		c := fake.NewClientBuilder().WithObjects(systemNetwork("migration", true)).Build()
		v := liveMigrationValidator{client: c}
		warns, err := v.validate(ctx, mcWith("migration"))
		Expect(err).NotTo(HaveOccurred())
		Expect(warns).To(BeNil())
	})

	It("rejects when SystemNetwork is not Ready", func() {
		c := fake.NewClientBuilder().WithObjects(systemNetwork("migration", false)).Build()
		v := liveMigrationValidator{client: c}
		_, err := v.validate(ctx, mcWith("migration"))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("is not Ready"))
	})

	It("rejects when SystemNetwork has no Ready condition at all", func() {
		sn := &unstructured.Unstructured{}
		sn.SetGroupVersionKind(systemNetworkGVK)
		sn.SetName("migration")
		c := fake.NewClientBuilder().WithObjects(sn).Build()
		v := liveMigrationValidator{client: c}
		_, err := v.validate(ctx, mcWith("migration"))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("is not Ready"))
	})
})

var _ = Describe("isSystemNetworkReady", func() {
	build := func(conditions []any) *unstructured.Unstructured {
		sn := &unstructured.Unstructured{}
		sn.SetGroupVersionKind(systemNetworkGVK)
		_ = unstructured.SetNestedSlice(sn.Object, conditions, "status", "conditions")
		return sn
	}

	DescribeTable("reads the Ready condition",
		func(obj *unstructured.Unstructured, expected bool) {
			Expect(isSystemNetworkReady(obj)).To(Equal(expected))
		},
		Entry("Ready=True",
			build([]any{map[string]any{"type": conditionTypeReady, "status": conditionStatusTrue}}),
			true),
		Entry("Ready=False",
			build([]any{map[string]any{"type": conditionTypeReady, "status": "False"}}),
			false),
		Entry("no Ready condition",
			build([]any{map[string]any{"type": "Other", "status": "True"}}),
			false),
		Entry("empty conditions list", build(nil), false),
		Entry("no status.conditions key",
			&unstructured.Unstructured{Object: map[string]any{}}, false),
	)
})
