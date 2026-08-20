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

package vd

import (
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/yaml"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const crdPath = "../../../../../../crds/virtualdisks.yaml"

// The phase list of the metric is maintained by hand, so keep it checked against the CRD enum:
// a phase known to the API but missing from the metric zeroes out every series of a disk in that
// phase, and the disk silently disappears from the phase dashboards.
var _ = Describe("Disk phase metric", func() {
	It("reports every phase of the VirtualDisk status enum", func() {
		data, err := os.ReadFile(crdPath)
		Expect(err).NotTo(HaveOccurred())

		var crd struct {
			Spec struct {
				Versions []struct {
					Name   string `json:"name"`
					Schema struct {
						OpenAPIV3Schema struct {
							Properties struct {
								Status struct {
									Properties struct {
										Phase struct {
											Enum []v1alpha2.DiskPhase `json:"enum"`
										} `json:"phase"`
									} `json:"properties"`
								} `json:"status"`
							} `json:"properties"`
						} `json:"openAPIV3Schema"`
					} `json:"schema"`
				} `json:"versions"`
			} `json:"spec"`
		}
		Expect(yaml.Unmarshal(data, &crd)).To(Succeed())
		Expect(crd.Spec.Versions).NotTo(BeEmpty())

		for _, version := range crd.Spec.Versions {
			enum := version.Schema.OpenAPIV3Schema.Properties.Status.Properties.Phase.Enum
			Expect(enum).NotTo(BeEmpty(), "no status phase enum in CRD version %s", version.Name)
			Expect(diskPhases).To(ContainElements(enum), "phases missing from the %s metric", MetricDiskStatusPhase)
		}
	})
})

func TestScraper(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "VirtualDisk Metrics Suite")
}
