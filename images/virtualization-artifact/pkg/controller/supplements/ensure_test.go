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

package supplements

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/datasource"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr/registrytoken"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("EnsureForPod", func() {
	const (
		sourceNamespace = "source-ns"
		sourceName      = "image-pull-secret"
	)

	var (
		ctx          context.Context
		fakeClient   client.Client
		supgen       Generator
		pod          *corev1.Pod
		dvcrSettings *dvcr.Settings
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

	BeforeEach(func() {
		ctx = context.Background()

		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())

		fakeClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: sourceName, Namespace: sourceNamespace},
				Data:       map[string][]byte{corev1.DockerConfigJsonKey: []byte(`{"auths":{}}`)},
				Type:       corev1.SecretTypeDockerConfigJson,
			},
		).Build()

		supgen = NewGenerator(annotations.CVIShortName, "cvi", "default", "33333333-3333-3333-3333-333333333333")
		pod = &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "importer", Namespace: "default"}}
		dvcrSettings = &dvcr.Settings{RegistryURL: "registry.example.com", TokenSigner: newSigner()}
	})

	It("copies the imagePullSecret from the namespace it lives in", func() {
		ds := &datasource.CABundle{
			Type: v1alpha2.DataSourceTypeContainerImage,
			ContainerImage: &datasource.ContainerRegistry{
				Image:           "registry.example.com/image:latest",
				ImagePullSecret: types.NamespacedName{Name: sourceName, Namespace: sourceNamespace},
			},
		}

		Expect(EnsureForPod(ctx, fakeClient, supgen, pod, ds, dvcrSettings, nil)).To(Succeed())

		copied := &corev1.Secret{}
		Expect(fakeClient.Get(ctx, supgen.ImagePullSecret(), copied)).To(Succeed())
		Expect(copied.Type).To(Equal(corev1.SecretTypeDockerConfigJson))
		Expect(copied.Data).To(HaveKey(corev1.DockerConfigJsonKey))
	})

	It("fails with a readable error when the imagePullSecret does not exist", func() {
		ds := &datasource.CABundle{
			Type: v1alpha2.DataSourceTypeContainerImage,
			ContainerImage: &datasource.ContainerRegistry{
				Image:           "registry.example.com/image:latest",
				ImagePullSecret: types.NamespacedName{Name: "missing", Namespace: sourceNamespace},
			},
		}

		err := EnsureForPod(ctx, fakeClient, supgen, pod, ds, dvcrSettings, nil)
		Expect(err).To(MatchError(ContainSubstring("the Secret source-ns/missing not found")))
	})
})
