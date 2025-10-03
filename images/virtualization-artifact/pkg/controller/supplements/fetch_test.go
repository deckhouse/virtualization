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
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestFetch(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Fetch Suite")
}

var _ = Describe("FetchSupplement", func() {
	var (
		ctx    context.Context
		scheme *runtime.Scheme
		gen    Generator
	)

	BeforeEach(func() {
		ctx = context.Background()

		scheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())

		gen = NewGenerator("vi", "test-image", "default", "12345678-1234-1234-1234-123456789abc")
	})

	Context("when resource exists with new naming", func() {
		It("should fetch the resource successfully", func() {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "d8v-vi-importer-test-image-12345678-1234-1234-1234-123456789abc",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "importer",
						Image: "importer:latest",
					}},
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(pod).
				Build()

			result := &corev1.Pod{}
			fetchedPod, err := FetchSupplement(ctx, fakeClient, gen, SupplementImporterPod, result)

			Expect(err).NotTo(HaveOccurred())
			Expect(fetchedPod).NotTo(BeNil())
			Expect(fetchedPod.Name).To(Equal(pod.Name))
			Expect(fetchedPod.Namespace).To(Equal(pod.Namespace))
		})
	})

	Context("when resource exists with legacy naming", func() {
		It("should fetch the resource from legacy naming as fallback", func() {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vi-importer-test-image",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "importer",
						Image: "importer:latest",
					}},
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(pod).
				Build()

			result := &corev1.Pod{}
			fetchedPod, err := FetchSupplement(ctx, fakeClient, gen, SupplementImporterPod, result)

			Expect(err).NotTo(HaveOccurred())
			Expect(fetchedPod).NotTo(BeNil())
			Expect(fetchedPod.Name).To(Equal(pod.Name))
			Expect(fetchedPod.Namespace).To(Equal(pod.Namespace))
		})
	})

	Context("when resource exists in both new and legacy naming", func() {
		It("should prefer the new naming", func() {
			newPod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "d8v-vi-importer-test-image-12345678-1234-1234-1234-123456789abc",
					Namespace: "default",
					Labels:    map[string]string{"version": "new"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "importer",
						Image: "importer:v2",
					}},
				},
			}

			legacyPod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vi-importer-test-image",
					Namespace: "default",
					Labels:    map[string]string{"version": "legacy"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "importer",
						Image: "importer:v1",
					}},
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(newPod, legacyPod).
				Build()

			result := &corev1.Pod{}
			fetchedPod, err := FetchSupplement(ctx, fakeClient, gen, SupplementImporterPod, result)

			Expect(err).NotTo(HaveOccurred())
			Expect(fetchedPod).NotTo(BeNil())
			Expect(fetchedPod.Name).To(Equal(newPod.Name))
			Expect(fetchedPod.Labels["version"]).To(Equal("new"))
		})
	})

	Context("when resource does not exist", func() {
		It("should return nil without error", func() {
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				Build()

			result := &corev1.Pod{}
			fetchedPod, err := FetchSupplement(ctx, fakeClient, gen, SupplementImporterPod, result)

			Expect(err).NotTo(HaveOccurred())
			Expect(fetchedPod).To(BeNil())
		})
	})

	Context("with different supplement types", func() {
		It("should work with UploaderService", func() {
			svc := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "d8v-vi-test-image-12345678-1234-1234-1234-123456789abc",
					Namespace: "default",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Port: 8080,
					}},
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(svc).
				Build()

			result := &corev1.Service{}
			fetchedSvc, err := FetchSupplement(ctx, fakeClient, gen, SupplementUploaderService, result)

			Expect(err).NotTo(HaveOccurred())
			Expect(fetchedSvc).NotTo(BeNil())
			Expect(fetchedSvc.Name).To(Equal(svc.Name))
		})

		It("should work with PersistentVolumeClaim", func() {
			pvc := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vi-test-image-12345678-1234-1234-1234-123456789abc",
					Namespace: "default",
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(pvc).
				Build()

			result := &corev1.PersistentVolumeClaim{}
			fetchedPVC, err := FetchSupplement(ctx, fakeClient, gen, SupplementPVC, result)

			Expect(err).NotTo(HaveOccurred())
			Expect(fetchedPVC).NotTo(BeNil())
			Expect(fetchedPVC.Name).To(Equal(pvc.Name))
		})
	})

	Context("when client returns other errors", func() {
		It("should propagate the error", func() {
			fakeClient := &errorClient{
				Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
			}

			result := &corev1.Pod{}
			_, err := FetchSupplement(ctx, fakeClient, gen, SupplementImporterPod, result)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("internal error"))
		})
	})
})

// errorClient is a client that always returns an error (not NotFound)
type errorClient struct {
	client.Client
}

func (e *errorClient) Get(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
	return &testError{message: "internal error"}
}

type testError struct {
	message string
}

func (e *testError) Error() string {
	return e.message
}
