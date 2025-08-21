/*
Copyright 2024 Flant JSC

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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service/restorer/common"
)

type ProvisionerTestArgs struct {
	mode common.OperationMode

	useDifferentVM bool
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
			handler = NewProvisionerHandler(fakeClient, args.mode, secret, uid)
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
			mode: common.StrictRestoreMode,

			secretExists:   true,
			secretDiffData: true,

			failValidation: true,
			failProcess:    true,

			shouldBeCreated: false,
		}),
		Entry("secret exists; same data", ProvisionerTestArgs{
			mode: common.StrictRestoreMode,

			secretExists:   true,
			secretDiffData: false,

			failValidation: false,
			failProcess:    false,

			shouldBeCreated: false,
		}),
		Entry("secret doesn't exist", ProvisionerTestArgs{
			mode: common.StrictRestoreMode,

			secretExists:   false,
			secretDiffData: false,

			failValidation: false,
			failProcess:    false,

			shouldBeCreated: true,
		}),
	)
})
