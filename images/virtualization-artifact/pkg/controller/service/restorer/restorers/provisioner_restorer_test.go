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

package restorer

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type ProvisionerTestArgs struct {
	secretExists   bool
	secretDiffData bool

	failValidation bool
	failProcess    bool

	shouldBeCreated bool
}

func TestRestorers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Module Restorers Test Suite")
}

var _ = Describe("ProvisionerRestorer", func() {
	var (
		ctx context.Context
		err error

		uid       string
		name      string
		namespace string

		intercept     interceptor.Funcs
		secretCreated bool

		objects    []client.Object
		secret     corev1.Secret
		handler    *ProvisionerHandler
		fakeClient client.WithWatch
	)

	BeforeEach(func() {
		ctx = context.Background()
		uid = "0000-1111-2222-4444"
		name = "test-secret"
		namespace = "default"
		secretCreated = false

		objects = []client.Object{}

		secret = corev1.Secret{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Secret",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Data:       map[string][]byte{"data": []byte("data")},
		}

		intercept = interceptor.Funcs{
			Create: func(_ context.Context, _ client.WithWatch, obj client.Object, _ ...client.CreateOption) error {
				if obj.GetName() == secret.Name {
					_, ok := obj.(*corev1.Secret)
					Expect(ok).To(BeTrue())
					secretCreated = true
				}

				return nil
			},
		}
	})

	DescribeTable("restore",
		func(args ProvisionerTestArgs) {
			if args.secretDiffData {
				secret.Data = map[string][]byte{"data": []byte("different-data")}
			}

			if args.secretExists {
				objects = append(objects, &secret)
			}

			fakeClient, err = testutil.NewFakeClientWithInterceptorWithObjects(intercept, objects...)
			Expect(err).ToNot(HaveOccurred())
			Expect(fakeClient).ToNot(BeNil())

			secret.Data = map[string][]byte{"data": []byte("data")}
			handler = NewProvisionerHandler(fakeClient, secret, uid)
			Expect(handler).ToNot(BeNil())

			err = handler.ValidateRestore(ctx)
			if args.failValidation {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
			}

			err = handler.ProcessRestore(ctx)
			if args.failProcess {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
			}

			Expect(secretCreated).To(Equal(args.shouldBeCreated))
		},
		Entry("secret exists; different data", ProvisionerTestArgs{
			secretExists:   true,
			secretDiffData: true,

			failValidation: true,
			failProcess:    true,

			shouldBeCreated: false,
		}),
		Entry("secret exists; same data", ProvisionerTestArgs{
			secretExists:   true,
			secretDiffData: false,

			failValidation: false,
			failProcess:    false,

			shouldBeCreated: false,
		}),
		Entry("secret doesn't exist", ProvisionerTestArgs{
			secretExists:   false,
			secretDiffData: false,

			failValidation: false,
			failProcess:    false,

			shouldBeCreated: true,
		}),
	)

	Describe("Override", func() {
		var rules []v1alpha2.NameReplacement

		BeforeEach(func() {
			rules = []v1alpha2.NameReplacement{
				{
					From: v1alpha2.NameReplacementFrom{
						Kind: "Secret",
						Name: name,
					},
					To: "new-secret-name",
				},
			}

			fakeClient, err = testutil.NewFakeClientWithInterceptorWithObjects(intercept)
			Expect(err).ToNot(HaveOccurred())

			handler = NewProvisionerHandler(fakeClient, secret, uid)
		})

		It("should override secret name", func() {
			handler.Override(rules)
			Expect(handler.secret.Name).To(Equal("new-secret-name"))
		})

		It("should not override non-matching names", func() {
			nonMatchingRules := []v1alpha2.NameReplacement{
				{
					From: v1alpha2.NameReplacementFrom{
						Kind: "Secret",
						Name: "different-secret",
					},
					To: "should-not-apply",
				},
			}

			originalName := handler.secret.Name
			handler.Override(nonMatchingRules)
			Expect(handler.secret.Name).To(Equal(originalName))
		})
	})
})
