/*
Copyright 2025 Flant JSC

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

package supplements

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"
	kvalidation "k8s.io/apimachinery/pkg/util/validation"
)

var _ = Describe("Generator", func() {
	var (
		gen       Generator
		prefix    string
		namespace string
		uid       types.UID
	)

	BeforeEach(func() {
		prefix = "vi"
		namespace = "default"
		uid = types.UID("12345678-1234-1234-1234-123456789012")
	})

	Context("Name shortening", func() {
		DescribeTable("should handle short names without truncation",
			func(method func(Generator) types.NamespacedName, expectedPrefix string) {
				name := "test-image"
				gen = NewGenerator(prefix, name, namespace, uid)
				result := method(gen)

				Expect(result.Name).To(HavePrefix("d8v-"))
				Expect(result.Name).To(ContainSubstring(expectedPrefix))
				Expect(result.Name).To(ContainSubstring(name))
				Expect(result.Name).To(HaveSuffix(string(uid)))
			},
			Entry("DVCRAuthSecret", func(g Generator) types.NamespacedName { return g.DVCRAuthSecret() }, "dvcr-auth"),
			Entry("DVCRAuthSecretForDV", func(g Generator) types.NamespacedName { return g.DVCRAuthSecretForDV() }, "dvcr-auth-dv"),
			Entry("DVCRCABundleConfigMapForDV", func(g Generator) types.NamespacedName { return g.DVCRCABundleConfigMapForDV() }, "dvcr-ca"),
			Entry("CABundleConfigMap", func(g Generator) types.NamespacedName { return g.CABundleConfigMap() }, "ca"),
			Entry("ImagePullSecret", func(g Generator) types.NamespacedName { return g.ImagePullSecret() }, "pull-image"),
			Entry("ImporterPod", func(g Generator) types.NamespacedName { return g.ImporterPod() }, "importer"),
			Entry("BounderPod", func(g Generator) types.NamespacedName { return g.BounderPod() }, "bounder"),
			Entry("UploaderPod", func(g Generator) types.NamespacedName { return g.UploaderPod() }, "uploader"),
			Entry("UploaderService", func(g Generator) types.NamespacedName { return g.UploaderService() }, "vi"),
			Entry("UploaderIngress", func(g Generator) types.NamespacedName { return g.UploaderIngress() }, "vi"),
			Entry("UploaderTLSSecret", func(g Generator) types.NamespacedName { return g.UploaderTLSSecretForIngress() }, "tls"),
			Entry("DataVolume", func(g Generator) types.NamespacedName { return g.DataVolume() }, "vi"),
			Entry("PersistentVolumeClaim", func(g Generator) types.NamespacedName { return g.PersistentVolumeClaim() }, "vi"),
			Entry("NetworkPolicy", func(g Generator) types.NamespacedName { return g.NetworkPolicy() }, "vi"),
			Entry("CommonSupplement", func(g Generator) types.NamespacedName { return g.CommonSupplement() }, "vi"),
		)

		It("should generate legacy snapshot supplement name without prefix or UID", func() {
			name := "test-snapshot"
			gen = NewGenerator("vms", name, namespace, uid)
			result := gen.LegacySnapshotSupplement()

			Expect(result.Name).To(Equal(name))
			Expect(result.Namespace).To(Equal(namespace))
		})

		DescribeTable("should truncate long names to respect limits",
			func(method func(Generator) types.NamespacedName, maxLength int) {
				name := strings.Repeat("very-long-resource-name-", 30)
				gen = NewGenerator(prefix, name, namespace, uid)
				result := method(gen)

				Expect(len(result.Name)).To(BeNumerically("<=", maxLength))
				Expect(result.Name).To(HavePrefix("d8v-"))
				Expect(result.Name).To(ContainSubstring(string(uid)))
			},
			Entry("DVCRAuthSecret - 253 limit", func(g Generator) types.NamespacedName { return g.DVCRAuthSecret() }, kvalidation.DNS1123SubdomainMaxLength),
			Entry("DVCRAuthSecretForDV - 253 limit", func(g Generator) types.NamespacedName { return g.DVCRAuthSecretForDV() }, kvalidation.DNS1123SubdomainMaxLength),
			Entry("DVCRCABundleConfigMapForDV - 253 limit", func(g Generator) types.NamespacedName { return g.DVCRCABundleConfigMapForDV() }, kvalidation.DNS1123SubdomainMaxLength),
			Entry("CABundleConfigMap - 253 limit", func(g Generator) types.NamespacedName { return g.CABundleConfigMap() }, kvalidation.DNS1123SubdomainMaxLength),
			Entry("ImagePullSecret - 253 limit", func(g Generator) types.NamespacedName { return g.ImagePullSecret() }, kvalidation.DNS1123SubdomainMaxLength),
			Entry("ImporterPod - 253 limit", func(g Generator) types.NamespacedName { return g.ImporterPod() }, kvalidation.DNS1123SubdomainMaxLength),
			Entry("BounderPod - 253 limit", func(g Generator) types.NamespacedName { return g.BounderPod() }, kvalidation.DNS1123SubdomainMaxLength),
			Entry("UploaderPod - 253 limit", func(g Generator) types.NamespacedName { return g.UploaderPod() }, kvalidation.DNS1123SubdomainMaxLength),
			Entry("UploaderService - 63 limit", func(g Generator) types.NamespacedName { return g.UploaderService() }, kvalidation.DNS1123LabelMaxLength),
			Entry("UploaderIngress - 253 limit", func(g Generator) types.NamespacedName { return g.UploaderIngress() }, kvalidation.DNS1123SubdomainMaxLength),
			Entry("UploaderTLSSecret - 253 limit", func(g Generator) types.NamespacedName { return g.UploaderTLSSecretForIngress() }, kvalidation.DNS1123SubdomainMaxLength),
			Entry("DataVolume - 253 limit", func(g Generator) types.NamespacedName { return g.DataVolume() }, kvalidation.DNS1123SubdomainMaxLength),
			Entry("PersistentVolumeClaim - 253 limit", func(g Generator) types.NamespacedName { return g.PersistentVolumeClaim() }, kvalidation.DNS1123SubdomainMaxLength),
			Entry("NetworkPolicy - 253 limit", func(g Generator) types.NamespacedName { return g.NetworkPolicy() }, kvalidation.DNS1123SubdomainMaxLength),
			Entry("CommonSupplement - 253 limit", func(g Generator) types.NamespacedName { return g.CommonSupplement() }, kvalidation.DNS1123SubdomainMaxLength),
		)
	})
})
