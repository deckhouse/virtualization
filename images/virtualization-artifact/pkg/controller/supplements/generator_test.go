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
		gen       *Generator
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
			func(method func(*Generator) types.NamespacedName, expectedPrefix string) {
				name := "test-image"
				gen = NewGenerator(prefix, name, namespace, uid)
				result := method(gen)

				Expect(result.Name).To(HavePrefix("d8v-"))
				Expect(result.Name).To(ContainSubstring(expectedPrefix))
				Expect(result.Name).To(ContainSubstring(name))
				Expect(result.Name).To(HaveSuffix(string(uid)))
			},
			Entry("DVCRAuthSecret", (*Generator).DVCRAuthSecret, "dvcr-auth"),
			Entry("DVCRAuthSecretForDV", (*Generator).DVCRAuthSecretForDV, "dvcr-auth"),
			Entry("DVCRCABundleConfigMapForDV", (*Generator).DVCRCABundleConfigMapForDV, "dvcr-ca"),
			Entry("CABundleConfigMap", (*Generator).CABundleConfigMap, "ca"),
			Entry("ImagePullSecret", (*Generator).ImagePullSecret, "pull-image"),
			Entry("ImporterPod", (*Generator).ImporterPod, "importer"),
			Entry("BounderPod", (*Generator).BounderPod, "bounder"),
			Entry("UploaderPod", (*Generator).UploaderPod, "uploader"),
			Entry("UploaderService", (*Generator).UploaderService, "vi"),
			Entry("UploaderIngress", (*Generator).UploaderIngress, "vi"),
			Entry("UploaderTLSSecret", (*Generator).UploaderTLSSecretForIngress, "tls"),
			Entry("DataVolume", (*Generator).DataVolume, "vi"),
			Entry("PersistentVolumeClaim", (*Generator).PersistentVolumeClaim, "vi"),
			Entry("NetworkPolicy", (*Generator).NetworkPolicy, "vi"),
		)

		DescribeTable("should truncate long names to respect limits",
			func(method func(*Generator) types.NamespacedName, maxLength int) {
				name := strings.Repeat("very-long-resource-name-", 30)
				gen = NewGenerator(prefix, name, namespace, uid)
				result := method(gen)

				Expect(len(result.Name)).To(BeNumerically("<=", maxLength))
				Expect(result.Name).To(HavePrefix("d8v-"))
				Expect(result.Name).To(ContainSubstring(string(uid)))
			},
			Entry("DVCRAuthSecret - 253 limit", (*Generator).DVCRAuthSecret, kvalidation.DNS1123SubdomainMaxLength),
			Entry("DVCRAuthSecretForDV - 253 limit", (*Generator).DVCRAuthSecretForDV, kvalidation.DNS1123SubdomainMaxLength),
			Entry("DVCRCABundleConfigMapForDV - 253 limit", (*Generator).DVCRCABundleConfigMapForDV, kvalidation.DNS1123SubdomainMaxLength),
			Entry("CABundleConfigMap - 253 limit", (*Generator).CABundleConfigMap, kvalidation.DNS1123SubdomainMaxLength),
			Entry("ImagePullSecret - 253 limit", (*Generator).ImagePullSecret, kvalidation.DNS1123SubdomainMaxLength),
			Entry("ImporterPod - 253 limit", (*Generator).ImporterPod, kvalidation.DNS1123SubdomainMaxLength),
			Entry("BounderPod - 253 limit", (*Generator).BounderPod, kvalidation.DNS1123SubdomainMaxLength),
			Entry("UploaderPod - 253 limit", (*Generator).UploaderPod, kvalidation.DNS1123SubdomainMaxLength),
			Entry("UploaderService - 63 limit", (*Generator).UploaderService, kvalidation.DNS1123LabelMaxLength),
			Entry("UploaderIngress - 253 limit", (*Generator).UploaderIngress, kvalidation.DNS1123SubdomainMaxLength),
			Entry("UploaderTLSSecret - 253 limit", (*Generator).UploaderTLSSecretForIngress, kvalidation.DNS1123SubdomainMaxLength),
			Entry("DataVolume - 253 limit", (*Generator).DataVolume, kvalidation.DNS1123SubdomainMaxLength),
			Entry("PersistentVolumeClaim - 253 limit", (*Generator).PersistentVolumeClaim, kvalidation.DNS1123SubdomainMaxLength),
			Entry("NetworkPolicy - 253 limit", (*Generator).NetworkPolicy, kvalidation.DNS1123SubdomainMaxLength),
		)
	})
})

