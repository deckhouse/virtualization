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

package uploader

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/datasource"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr/registrytoken"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestUploader(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Uploader")
}

var _ = Describe("Uploader", func() {
	var (
		ctx        context.Context
		fakeClient client.Client
		vi         *v1alpha2.VirtualImage
		supgen     supplements.Generator
	)

	newSigner := func() *registrytoken.Signer {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		Expect(err).ToNot(HaveOccurred())
		der, err := x509.MarshalPKCS8PrivateKey(key)
		Expect(err).ToNot(HaveOccurred())
		signer, err := registrytoken.NewSignerFromPEM(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
		Expect(err).ToNot(HaveOccurred())
		return signer
	}

	newUploader := func(ingressHost string) Uploader {
		dvcrSettings := &dvcr.Settings{
			RegistryURL: "registry.example.com",
			TokenSigner: newSigner(),
			UploaderIngressSettings: dvcr.UploaderIngressSettings{
				Host: ingressHost,
			},
		}

		return NewUploader(
			fakeClient,
			dvcrSettings,
			"uploader-image",
			corev1.ResourceRequirements{},
			string(corev1.PullIfNotPresent),
			"1",
			"vi-controller",
			featuregates.Default(),
		)
	}

	apply := func(uploader Uploader) {
		var settings Settings
		ApplyDVCRDestinationSettings(&settings, &dvcr.Settings{RegistryURL: "registry.example.com"}, supgen, "vi/default/vi:latest")

		err := uploader.Apply(ctx, vi, supgen, settings,
			datasource.NewCABundleForVMI(vi.Namespace, vi.Spec.DataSource),
			WithSystemNodeToleration(),
		)
		Expect(err).ToNot(HaveOccurred())
	}

	ingresses := func() []netv1.Ingress {
		list := &netv1.IngressList{}
		Expect(fakeClient.List(ctx, list)).To(Succeed())
		return list.Items
	}

	BeforeEach(func() {
		ctx = context.Background()

		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(netv1.AddToScheme(scheme)).To(Succeed())
		Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

		fakeClient = fake.NewClientBuilder().WithScheme(scheme).
			WithInterceptorFuncs(interceptor.Funcs{
				Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
					if patch != client.Apply {
						return c.Patch(ctx, obj, patch, opts...)
					}

					// The fake client does not implement server-side apply. The
					// objects applied here are fully specified desired states, so
					// create-or-overwrite reproduces what the API server would do.
					err := c.Create(ctx, obj)
					if !k8serrors.IsAlreadyExists(err) {
						return err
					}

					existing, ok := obj.DeepCopyObject().(client.Object)
					Expect(ok).To(BeTrue())
					if err = c.Get(ctx, client.ObjectKeyFromObject(obj), existing); err != nil {
						return err
					}
					obj.SetResourceVersion(existing.GetResourceVersion())

					return c.Update(ctx, obj)
				},
			}).Build()

		vi = &v1alpha2.VirtualImage{
			TypeMeta: metav1.TypeMeta{
				APIVersion: v1alpha2.SchemeGroupVersion.String(),
				Kind:       v1alpha2.VirtualImageKind,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vi",
				Namespace: "default",
				UID:       "11111111-1111-1111-1111-111111111111",
			},
			Spec: v1alpha2.VirtualImageSpec{
				DataSource: v1alpha2.VirtualImageDataSource{Type: v1alpha2.DataSourceTypeUpload},
			},
		}
		supgen = supplements.NewGenerator(annotations.VIShortName, vi.Name, vi.Namespace, vi.UID)
	})

	Context("a public host is configured", func() {
		It("publishes the upload on an Ingress", func() {
			uploader := newUploader("virtualization.example.com")
			apply(uploader)

			Expect(ingresses()).To(HaveLen(1))

			exposure, err := uploader.GetExposure(ctx, supgen)
			Expect(err).ToNot(HaveOccurred())
			Expect(exposure.Required).To(BeTrue())
			Expect(exposure.Exists).To(BeTrue())
			Expect(exposure.Ensured()).To(BeTrue())
			Expect(exposure.UploadPath).ToNot(BeEmpty())
			Expect(exposure.UploadURL).To(HavePrefix("http://virtualization.example.com/upload/"))
		})

		It("keeps the published path across reconciles", func() {
			uploader := newUploader("virtualization.example.com")
			apply(uploader)

			before, err := uploader.GetExposure(ctx, supgen)
			Expect(err).ToNot(HaveOccurred())

			apply(uploader)
			Expect(uploader.EnsureExposure(ctx, vi, supgen)).To(Succeed())

			after, err := uploader.GetExposure(ctx, supgen)
			Expect(err).ToNot(HaveOccurred())
			Expect(after.UploadPath).To(Equal(before.UploadPath))
			Expect(after.UploadURL).To(Equal(before.UploadURL))
		})

		It("rebuilds an Ingress that lost its upload-path annotation", func() {
			uploader := newUploader("virtualization.example.com")
			apply(uploader)

			ing := ingresses()[0]
			delete(ing.Annotations, annotations.AnnUploadPath)
			delete(ing.Annotations, annotations.AnnUploadURL)
			Expect(fakeClient.Update(ctx, &ing)).To(Succeed())

			Expect(uploader.EnsureExposure(ctx, vi, supgen)).To(Succeed())

			exposure, err := uploader.GetExposure(ctx, supgen)
			Expect(err).ToNot(HaveOccurred())
			// A fresh path is generated and written to the object itself, so the URL
			// reported to the user matches the one actually served.
			Expect(exposure.UploadPath).ToNot(BeEmpty())
			Expect(exposure.UploadURL).To(HaveSuffix(exposure.UploadPath))

			repaired := ingresses()[0]
			Expect(repaired.Spec.Rules).To(HaveLen(1))
			Expect(repaired.Spec.Rules[0].HTTP.Paths[0].Path).To(Equal(exposure.UploadPath))
		})
	})

	Context("no public host is configured", func() {
		It("creates the uploader without an external exposure", func() {
			uploader := newUploader("")
			apply(uploader)

			pod, err := uploader.GetPod(ctx, supgen)
			Expect(err).ToNot(HaveOccurred())
			Expect(pod).ToNot(BeNil())

			svc, err := uploader.GetService(ctx, supgen)
			Expect(err).ToNot(HaveOccurred())
			Expect(svc).ToNot(BeNil())

			// Without a host an Ingress would be a catch-all rule serving a
			// malformed upload URL, so none is created: the upload goes through the
			// in-cluster Service URL.
			Expect(ingresses()).To(BeEmpty())
		})

		It("reports the exposure as not required, hence ensured", func() {
			uploader := newUploader("")
			apply(uploader)

			exposure, err := uploader.GetExposure(ctx, supgen)
			Expect(err).ToNot(HaveOccurred())
			Expect(exposure.Required).To(BeFalse())
			Expect(exposure.Exists).To(BeFalse())
			// The reconcilers gate uploader creation on Ensured(): a missing exposure
			// nobody is going to create must not restart the provisioning forever.
			Expect(exposure.Ensured()).To(BeTrue())
			Expect(exposure.UploadURL).To(BeEmpty())
		})

		It("keeps EnsureExposure a no-op", func() {
			uploader := newUploader("")
			apply(uploader)

			Expect(uploader.EnsureExposure(ctx, vi, supgen)).To(Succeed())
			Expect(ingresses()).To(BeEmpty())
		})
	})
})
