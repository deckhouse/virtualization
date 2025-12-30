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
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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
	var scheme *runtime.Scheme

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

		It("should return true when image exists", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/v2/":
					w.WriteHeader(http.StatusOK)
				case strings.HasPrefix(r.URL.Path, "/v2/") && strings.Contains(r.URL.Path, "/manifests/"):
					w.Header().Set("Docker-Content-Digest", "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")
					w.Header().Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
					w.Header().Set("Content-Length", "123")
					w.WriteHeader(http.StatusOK)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()

			client := fake.NewClientBuilder().WithScheme(scheme).Build()
			settings := &Settings{
				InsecureTLS: "true",
			}
			checker := NewImageChecker(client, settings)

			registryHost := strings.TrimPrefix(server.URL, "http://")
			imageURL := fmt.Sprintf("%s/vi/test:abc123", registryHost)

			exists, err := checker.CheckImageExists(context.Background(), imageURL)

			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeTrue())
		})

		It("should return false when image does not exist", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/v2/":
					w.WriteHeader(http.StatusOK)
				case strings.HasPrefix(r.URL.Path, "/v2/") && strings.Contains(r.URL.Path, "/manifests/"):
					w.WriteHeader(http.StatusNotFound)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()

			client := fake.NewClientBuilder().WithScheme(scheme).Build()
			settings := &Settings{
				InsecureTLS: "true",
			}
			checker := NewImageChecker(client, settings)

			registryHost := strings.TrimPrefix(server.URL, "http://")
			imageURL := fmt.Sprintf("%s/vi/test:notfound", registryHost)

			exists, err := checker.CheckImageExists(context.Background(), imageURL)

			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeFalse())
		})
	})
})
