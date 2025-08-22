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

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service/restorer/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VMBlockDeviceAttachmentTestArgs struct {
	mode common.OperationMode

	vmbdaExists       bool
	vmbdaUsedByDiffVM bool

	failValidation bool
	failProcess    bool

	shouldBeDeleted bool
	shouldBeCreated bool
}

var _ = Describe("VMBlockDeviceAttachmentRestorer", func() {
	var (
		ctx context.Context
		err error

		uid       string
		vm        string
		name      string
		namespace string

		intercept    interceptor.Funcs
		vmbdaDeleted bool
		vmbdaCreated bool

		objects    []client.Object
		vmbda      v1alpha2.VirtualMachineBlockDeviceAttachment
		handler    *VMBlockDeviceAttachmentHandler
		fakeClient client.WithWatch
	)

	BeforeEach(func() {
		ctx = context.Background()
		uid = "0000-1111-2222-4444"
		name = "test-vmbda"
		namespace = "default"
		vm = "test-vm"
		vmbdaDeleted = false
		vmbdaCreated = false

		objects = []client.Object{}
		vmbda = v1alpha2.VirtualMachineBlockDeviceAttachment{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec:       v1alpha2.VirtualMachineBlockDeviceAttachmentSpec{VirtualMachineName: vm},
		}

		intercept = interceptor.Funcs{
			Delete: func(_ context.Context, _ client.WithWatch, obj client.Object, _ ...client.DeleteOption) error {
				if obj.GetName() == vmbda.Name {
					_, ok := obj.(*v1alpha2.VirtualMachineBlockDeviceAttachment)
					Expect(ok).To(BeTrue())
					vmbdaDeleted = true
				}
				return nil
			},
			Create: func(_ context.Context, _ client.WithWatch, obj client.Object, _ ...client.CreateOption) error {
				if obj.GetName() == vmbda.Name {
					_, ok := obj.(*v1alpha2.VirtualMachineBlockDeviceAttachment)
					Expect(ok).To(BeTrue())
					vmbdaCreated = true
				}

				return nil
			},
		}
	})

	DescribeTable("restore",
		func(args VMBlockDeviceAttachmentTestArgs) {
			if args.vmbdaUsedByDiffVM {
				vmbda.Spec.VirtualMachineName = vm + "-2"
			}

			if args.vmbdaExists {
				objects = append(objects, &vmbda)
			}

			fakeClient, err = testutil.NewFakeClientWithInterceptorWithObjects(intercept, objects...)
			Expect(err).ToNot(HaveOccurred())
			Expect(fakeClient).ToNot(BeNil())

			vmbda.Spec.VirtualMachineName = vm
			handler = NewVMBlockDeviceAttachmentHandler(fakeClient, args.mode, vmbda, uid)
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

			Expect(vmbdaDeleted).To(Equal(args.shouldBeDeleted))
			Expect(vmbdaCreated).To(Equal(args.shouldBeCreated))
		},
		Entry("vmbda exists; used by different VM", VMBlockDeviceAttachmentTestArgs{
			mode: common.StrictRestoreMode,

			vmbdaExists:       true,
			vmbdaUsedByDiffVM: true,

			failValidation: true,
			failProcess:    true,

			shouldBeDeleted: false,
			shouldBeCreated: false,
		}),
		Entry("vmbda exists; not used by different VM", VMBlockDeviceAttachmentTestArgs{
			mode: common.StrictRestoreMode,

			vmbdaExists:       true,
			vmbdaUsedByDiffVM: false,

			failValidation: false,
			failProcess:    false,

			shouldBeDeleted: true,
			shouldBeCreated: true,
		}),
		Entry("vmbda doesn't exist", VMBlockDeviceAttachmentTestArgs{
			mode: common.StrictRestoreMode,

			vmbdaExists:       false,
			vmbdaUsedByDiffVM: false,

			failValidation: false,
			failProcess:    false,

			shouldBeDeleted: false,
			shouldBeCreated: true,
		}),
	)
})
