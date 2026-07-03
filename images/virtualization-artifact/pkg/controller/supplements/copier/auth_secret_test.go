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

package copier

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/dvcr/registrytoken"
)

func TestCopier(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Copier Suite")
}

func newTestSigner() *registrytoken.Signer {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	Expect(err).ToNot(HaveOccurred())
	der, err := x509.MarshalPKCS8PrivateKey(key)
	Expect(err).ToNot(HaveOccurred())
	signer, err := registrytoken.NewSignerFromPEM(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	Expect(err).ToNot(HaveOccurred())
	return signer
}

var _ = Describe("Scoped token secret refresh", func() {
	var (
		ctx    context.Context
		scheme *runtime.Scheme
		signer *registrytoken.Signer
		dest   types.NamespacedName
		access []registrytoken.Access
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		signer = newTestSigner()
		dest = types.NamespacedName{Name: "dvcr-auth", Namespace: "default"}
		access = []registrytoken.Access{{Type: "repository", Name: "cvi/img", Actions: []string{"pull", "push"}}}
	})

	authSecret := func() AuthSecret {
		return AuthSecret{Secret: Secret{Destination: dest}}
	}

	It("creates the secret with an expiry annotation", func() {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		Expect(authSecret().CreateScopedTokenCDI(ctx, c, signer, access)).To(Succeed())

		got := &corev1.Secret{}
		Expect(c.Get(ctx, dest, got)).To(Succeed())
		Expect(got.Data).To(HaveKey("secretKey"))
		Expect(got.Annotations).To(HaveKey(scopedTokenExpiryAnnotation))
	})

	It("does not re-mint while the token is still fresh", func() {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		Expect(authSecret().CreateScopedTokenCDI(ctx, c, signer, access)).To(Succeed())
		first := &corev1.Secret{}
		Expect(c.Get(ctx, dest, first)).To(Succeed())

		Expect(authSecret().CreateScopedTokenCDI(ctx, c, signer, access)).To(Succeed())
		second := &corev1.Secret{}
		Expect(c.Get(ctx, dest, second)).To(Succeed())
		Expect(second.Data["secretKey"]).To(Equal(first.Data["secretKey"]))
	})

	It("re-mints when the token is close to expiring", func() {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		Expect(authSecret().CreateScopedTokenCDI(ctx, c, signer, access)).To(Succeed())
		stale := &corev1.Secret{}
		Expect(c.Get(ctx, dest, stale)).To(Succeed())
		before := stale.Data["secretKey"]

		stale.Annotations[scopedTokenExpiryAnnotation] = time.Now().Add(time.Minute).Format(time.RFC3339)
		Expect(c.Update(ctx, stale)).To(Succeed())

		Expect(authSecret().CreateScopedTokenCDI(ctx, c, signer, access)).To(Succeed())
		after := &corev1.Secret{}
		Expect(c.Get(ctx, dest, after)).To(Succeed())
		Expect(after.Data["secretKey"]).ToNot(Equal(before))
	})

	It("scopedTokenValidFor returns zero for missing, unparseable or past expiry", func() {
		now := time.Now()
		Expect(scopedTokenValidFor(&corev1.Secret{}, now)).To(BeZero())
		bad := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{scopedTokenExpiryAnnotation: "garbage"}}}
		Expect(scopedTokenValidFor(bad, now)).To(BeZero())
		past := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{scopedTokenExpiryAnnotation: now.Add(-time.Hour).Format(time.RFC3339)}}}
		Expect(scopedTokenValidFor(past, now)).To(BeZero())
	})
})
