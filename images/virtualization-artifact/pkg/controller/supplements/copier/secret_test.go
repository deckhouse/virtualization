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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("Secret CopyOrUpdate", func() {
	var (
		ctx  context.Context
		c    client.Client
		src  types.NamespacedName
		dst  types.NamespacedName
		cp   Secret
		data map[string][]byte
	)

	fetch := func(key types.NamespacedName) *corev1.Secret {
		secret := &corev1.Secret{}
		Expect(c.Get(ctx, key, secret)).To(Succeed())
		return secret
	}

	BeforeEach(func() {
		ctx = context.Background()
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())

		src = types.NamespacedName{Namespace: "d8-virtualization", Name: "module-registry"}
		dst = types.NamespacedName{Namespace: "user-ns", Name: "copy"}
		data = map[string][]byte{".dockerconfigjson": []byte(`{"auths":{}}`)}

		c = fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Namespace: src.Namespace, Name: src.Name},
			Data:       data,
			Type:       corev1.SecretTypeDockerConfigJson,
		}).Build()

		cp = Secret{Source: src, Destination: dst}
	})

	It("creates the destination secret from the source", func() {
		Expect(cp.CopyOrUpdate(ctx, c)).To(Succeed())
		got := fetch(dst)
		Expect(got.Data).To(Equal(data))
		Expect(got.Type).To(Equal(corev1.SecretTypeDockerConfigJson))
	})

	It("refreshes an existing destination after source rotation", func() {
		Expect(cp.CopyOrUpdate(ctx, c)).To(Succeed())

		rotated := fetch(src)
		rotated.Data = map[string][]byte{".dockerconfigjson": []byte(`{"auths":{"new":{}}}`)}
		Expect(c.Update(ctx, rotated)).To(Succeed())

		Expect(cp.CopyOrUpdate(ctx, c)).To(Succeed())
		Expect(fetch(dst).Data).To(Equal(rotated.Data))
	})

	It("adds a second pod owner as a non-controller reference", func() {
		firstOwner := metav1.OwnerReference{
			APIVersion: "v1", Kind: "Pod", Name: "source-pod", UID: "uid-1",
			Controller: ptr.To(true), BlockOwnerDeletion: ptr.To(true),
		}
		cp.OwnerReference = firstOwner
		Expect(cp.CopyOrUpdate(ctx, c)).To(Succeed())

		secondOwner := firstOwner
		secondOwner.Name = "target-pod"
		secondOwner.UID = "uid-2"
		cp.OwnerReference = secondOwner
		Expect(cp.CopyOrUpdate(ctx, c)).To(Succeed())

		owners := fetch(dst).OwnerReferences
		Expect(owners).To(HaveLen(2))
		Expect(*owners[0].Controller).To(BeTrue())
		Expect(*owners[1].Controller).To(BeFalse())
		Expect(owners[1].UID).To(Equal(secondOwner.UID))

		cp.OwnerReference = secondOwner
		Expect(cp.CopyOrUpdate(ctx, c)).To(Succeed())
		Expect(fetch(dst).OwnerReferences).To(HaveLen(2))
	})

	It("fails when the source secret is absent", func() {
		Expect(c.Delete(ctx, fetch(src))).To(Succeed())
		Expect(cp.CopyOrUpdate(ctx, c)).To(MatchError(ContainSubstring("not found")))
	})
})
