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

package dvcr

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestImageChecker(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ImageChecker Suite")
}

var _ = Describe("ImageChecker", func() {
	var (
		scheme *runtime.Scheme
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
	})

	Context("CheckImageExists", func() {
		It("should return error for empty imageURL", func() {
			client := fake.NewClientBuilder().WithScheme(scheme).Build()
			settings := &Settings{}
			checker := NewImageChecker(client, settings)

			exists, err := checker.CheckImageExists(context.Background(), "")

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("image URL is empty"))
			Expect(exists).To(BeFalse())
		})

		It("should return error for invalid image reference", func() {
			client := fake.NewClientBuilder().WithScheme(scheme).Build()
			settings := &Settings{}
			checker := NewImageChecker(client, settings)

			exists, err := checker.CheckImageExists(context.Background(), ":::invalid:::")

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse image reference"))
			Expect(exists).To(BeFalse())
		})

		It("should return error when auth secret is not found", func() {
			client := fake.NewClientBuilder().WithScheme(scheme).Build()
			settings := &Settings{
				AuthSecret:          "dvcr-auth",
				AuthSecretNamespace: "d8-virtualization",
			}
			checker := NewImageChecker(client, settings)

			exists, err := checker.CheckImageExists(context.Background(), "dvcr.example.com/vi/test:abc123")

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get auth secret"))
			Expect(exists).To(BeFalse())
		})

		It("should return error when certs secret is not found", func() {
			authSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dvcr-auth",
					Namespace: "d8-virtualization",
				},
				Type: corev1.SecretTypeDockerConfigJson,
				Data: map[string][]byte{
					".dockerconfigjson": []byte(`{"auths":{"dvcr.example.com":{"auth":"dGVzdDp0ZXN0"}}}`),
				},
			}

			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(authSecret).Build()
			settings := &Settings{
				AuthSecret:           "dvcr-auth",
				AuthSecretNamespace:  "d8-virtualization",
				CertsSecret:          "dvcr-certs",
				CertsSecretNamespace: "d8-virtualization",
			}
			checker := NewImageChecker(client, settings)

			exists, err := checker.CheckImageExists(context.Background(), "dvcr.example.com/vi/test:abc123")

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get certs secret"))
			Expect(exists).To(BeFalse())
		})

		It("should return error when ca.crt is missing in certs secret", func() {
			authSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dvcr-auth",
					Namespace: "d8-virtualization",
				},
				Type: corev1.SecretTypeDockerConfigJson,
				Data: map[string][]byte{
					".dockerconfigjson": []byte(`{"auths":{"dvcr.example.com":{"auth":"dGVzdDp0ZXN0"}}}`),
				},
			}
			certsSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dvcr-certs",
					Namespace: "d8-virtualization",
				},
				Data: map[string][]byte{},
			}

			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(authSecret, certsSecret).Build()
			settings := &Settings{
				AuthSecret:           "dvcr-auth",
				AuthSecretNamespace:  "d8-virtualization",
				CertsSecret:          "dvcr-certs",
				CertsSecretNamespace: "d8-virtualization",
			}
			checker := NewImageChecker(client, settings)

			exists, err := checker.CheckImageExists(context.Background(), "dvcr.example.com/vi/test:abc123")

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("ca.crt not found in secret"))
			Expect(exists).To(BeFalse())
		})
	})
})
