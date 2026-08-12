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
	k8sversion "k8s.io/apimachinery/pkg/util/version"

	mcapi "github.com/deckhouse/virtualization-controller/pkg/controller/moduleconfig/api"
	"github.com/deckhouse/virtualization-controller/pkg/version"
)

var _ = Describe("addedFeatureGates", func() {
	DescribeTable("reports the gates the update enables",
		func(current, desired, expected []string) {
			Expect(addedFeatureGates(current, desired)).To(Equal(expected))
		},
		Entry("no gates at all", []string(nil), []string(nil), []string(nil)),
		Entry("the first gate", []string(nil), []string{"SDN"}, []string{"SDN"}),
		Entry("an unchanged gate", []string{"SDN"}, []string{"SDN"}, []string(nil)),
		Entry("a removed gate", []string{"SDN"}, []string(nil), []string(nil)),
		Entry("a gate added next to an enabled one",
			[]string{"SDN"}, []string{"SDN", "HotplugCPUWithLiveMigration"},
			[]string{"HotplugCPUWithLiveMigration"}),
	)
})

var _ = Describe("featureGatesValidator", func() {
	const gate = inPlaceResizeFeatureGate

	newMC := func(gates ...string) *mcapi.ModuleConfig {
		values := make([]any, 0, len(gates))
		for _, g := range gates {
			values = append(values, g)
		}
		mc := &mcapi.ModuleConfig{}
		mc.Spec.Settings = mcapi.SettingsValues{"featureGates": values}
		return mc
	}

	// Nothing is locked: models an edition where hotplug gates can be enabled by hand.
	newValidator := func(kubernetesVersion string) featureGatesValidator {
		return featureGatesValidator{
			edition:           version.EditionEE,
			lockedToDisabled:  func(string) bool { return false },
			kubernetesVersion: k8sversion.MustParseGeneric(kubernetesVersion),
		}
	}

	// The webhook is registered for UPDATE only, see templates/virtualization-controller/validation-webhook.yaml:
	// enabling a gate always means adding it to the ModuleConfig the module already has.
	enable := func(v featureGatesValidator, gates ...string) error {
		_, err := v.ValidateUpdate(context.Background(), newMC(), newMC(gates...))
		return err
	}

	Context("kubernetes version", func() {
		It("allows the gate on Kubernetes 1.33", func() {
			Expect(enable(newValidator("v1.33.0"), gate)).To(Succeed())
		})

		It("rejects the gate on Kubernetes 1.32", func() {
			err := enable(newValidator("v1.32.13"), gate)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(gate))
			Expect(err.Error()).To(ContainSubstring("1.33"))
			Expect(err.Error()).To(ContainSubstring("1.32.13"))
			Expect(err.Error()).To(ContainSubstring("every node running virtual machines"))
		})

		It("ignores configs without the gate", func() {
			Expect(enable(newValidator("v1.32.13"), "HotplugCPUWithLiveMigration")).To(Succeed())
		})

		// The version comes from the KUBERNETES_VERSION env variable read at start up, so it is
		// missing only when the controller is wired up incorrectly.
		It("rejects the gate when the Kubernetes version is unknown", func() {
			v := newValidator("v1.33.0")
			v.kubernetesVersion = nil

			err := enable(v, gate)
			Expect(err).To(MatchError(errKubernetesVersionUnknown))
			Expect(err.Error()).To(ContainSubstring(gate))
		})
	})

	Context("kubernetes version for GPU", func() {
		It("allows the gate on Kubernetes 1.34", func() {
			Expect(enable(newValidator("v1.34.2"), gpuFeatureGate)).To(Succeed())
		})

		It("rejects the gate on Kubernetes 1.33", func() {
			err := enable(newValidator("v1.33.13"), gpuFeatureGate)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(gpuFeatureGate))
			Expect(err.Error()).To(ContainSubstring("1.34"))
			Expect(err.Error()).To(ContainSubstring("1.33.13"))
			Expect(err.Error()).To(ContainSubstring("resource.k8s.io/v1"))
		})

		It("rejects the gate when the Kubernetes version is unknown", func() {
			v := newValidator("v1.34.2")
			v.kubernetesVersion = nil

			err := enable(v, gpuFeatureGate)
			Expect(err).To(MatchError(errKubernetesVersionUnknown))
			Expect(err.Error()).To(ContainSubstring(gpuFeatureGate))
		})
	})

	Context("edition", func() {
		// Gates unavailable in an edition are locked to a false default, see pkg/featuregates.
		newCEValidator := func() featureGatesValidator {
			return featureGatesValidator{
				edition:           version.EditionCE,
				lockedToDisabled:  func(name string) bool { return name != "SDN" },
				kubernetesVersion: k8sversion.MustParseGeneric("v1.34.2"),
			}
		}

		It("rejects a gate that cannot be enabled in this edition", func() {
			err := enable(newCEValidator(), gate)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not available in the CE edition"))
			Expect(err.Error()).To(ContainSubstring(gate))
		})

		It("reports every unavailable gate, not just the first one", func() {
			err := enable(newCEValidator(), "HotplugCPUWithLiveMigration", "HotplugMemoryWithLiveMigration")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("HotplugCPUWithLiveMigration, HotplugMemoryWithLiveMigration"))
		})

		It("allows gates that are not locked", func() {
			Expect(enable(newCEValidator(), "SDN")).To(Succeed())
		})

		// The edition check runs first: on CE the Kubernetes version is irrelevant.
		It("reports the edition rather than the Kubernetes version", func() {
			v := newCEValidator()
			v.kubernetesVersion = k8sversion.MustParseGeneric("v1.32.13")

			err := enable(v, gate)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not available in the CE edition"))
		})
	})

	Context("gates already enabled", func() {
		It("allows editing a config where the gate is already enabled", func() {
			_, err := newValidator("v1.32.13").ValidateUpdate(context.Background(), newMC(gate), newMC(gate))
			Expect(err).NotTo(HaveOccurred())
		})

		It("allows removing the gate", func() {
			_, err := newValidator("v1.32.13").ValidateUpdate(context.Background(), newMC(gate), newMC())
			Expect(err).NotTo(HaveOccurred())
		})

		It("checks only the gates added by this update", func() {
			_, err := newValidator("v1.32.13").ValidateUpdate(context.Background(),
				newMC(gate), newMC(gate, "HotplugCPUWithLiveMigration"))
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
