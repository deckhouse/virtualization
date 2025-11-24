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
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service/restorer/common"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VMBlockDeviceAttachmentTestArgs struct {
	mode v1alpha2.SnapshotOperationMode

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
			TypeMeta: metav1.TypeMeta{
				Kind:       "VirtualMachineBlockDeviceAttachment",
				APIVersion: v1alpha2.SchemeGroupVersion.String(),
			},
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
			handler = NewVMBlockDeviceAttachmentHandler(fakeClient, vmbda, uid)
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
			mode: v1alpha2.SnapshotOperationModeStrict,

			vmbdaExists:       true,
			vmbdaUsedByDiffVM: true,

			failValidation: true,
			failProcess:    true,

			shouldBeDeleted: false,
			shouldBeCreated: false,
		}),
		Entry("vmbda exists; not used by different VM", VMBlockDeviceAttachmentTestArgs{
			mode: v1alpha2.SnapshotOperationModeStrict,

			vmbdaExists:       true,
			vmbdaUsedByDiffVM: false,

			failValidation: false,
			failProcess:    true,

			shouldBeDeleted: true,
			shouldBeCreated: false,
		}),
		Entry("vmbda doesn't exist", VMBlockDeviceAttachmentTestArgs{
			mode: v1alpha2.SnapshotOperationModeStrict,

			vmbdaExists:       false,
			vmbdaUsedByDiffVM: false,

			failValidation: false,
			failProcess:    false,

			shouldBeDeleted: false,
			shouldBeCreated: true,
		}),
		Entry("vmbda deletion completed; ready to create", VMBlockDeviceAttachmentTestArgs{
			mode: v1alpha2.SnapshotOperationModeStrict,

			vmbdaExists:       false,
			vmbdaUsedByDiffVM: false,

			failValidation: false,
			failProcess:    false,

			shouldBeDeleted: false,
			shouldBeCreated: true,
		}),
	)

	Describe("Two-phase deletion behavior", func() {
		It("should return ErrWaitingForDeletion on first call when VMBDA needs replacement", func() {
			objects = append(objects, &vmbda)

			fakeClient, err = testutil.NewFakeClientWithInterceptorWithObjects(intercept, objects...)
			Expect(err).ToNot(HaveOccurred())

			handler = NewVMBlockDeviceAttachmentHandler(fakeClient, vmbda, uid)

			err = handler.ProcessRestore(ctx)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, common.ErrWaitingForDeletion)).To(BeTrue())
			Expect(vmbdaDeleted).To(BeTrue())
			Expect(vmbdaCreated).To(BeFalse())
		})
	})

	Describe("Override", func() {
		var rules []v1alpha2.NameReplacement

		BeforeEach(func() {
			vmbda.Spec.BlockDeviceRef = v1alpha2.VMBDAObjectRef{
				Kind: v1alpha2.VMBDAObjectRefKindVirtualDisk,
				Name: "test-disk",
			}

			rules = []v1alpha2.NameReplacement{
				{
					From: v1alpha2.NameReplacementFrom{
						Kind: "VirtualMachineBlockDeviceAttachment",
						Name: name,
					},
					To: "new-vmbda-name",
				},
				{
					From: v1alpha2.NameReplacementFrom{
						Kind: "VirtualMachine",
						Name: vm,
					},
					To: "new-vm-name",
				},
				{
					From: v1alpha2.NameReplacementFrom{
						Kind: "VirtualDisk",
						Name: "test-disk",
					},
					To: "new-disk-name",
				},
			}

			fakeClient, err = testutil.NewFakeClientWithInterceptorWithObjects(intercept)
			Expect(err).ToNot(HaveOccurred())

			handler = NewVMBlockDeviceAttachmentHandler(fakeClient, vmbda, uid)
		})

		It("should override VMBDA name", func() {
			handler.Override(rules)
			Expect(handler.vmbda.Name).To(Equal("new-vmbda-name"))
		})

		It("should override VirtualMachine name", func() {
			handler.Override(rules)
			Expect(handler.vmbda.Spec.VirtualMachineName).To(Equal("new-vm-name"))
		})

		It("should override VirtualDisk name in BlockDeviceRef", func() {
			handler.Override(rules)
			Expect(handler.vmbda.Spec.BlockDeviceRef.Name).To(Equal("new-disk-name"))
		})

		It("should override ClusterVirtualImage name in BlockDeviceRef", func() {
			handler.vmbda.Spec.BlockDeviceRef.Kind = v1alpha2.VMBDAObjectRefKindClusterVirtualImage
			handler.vmbda.Spec.BlockDeviceRef.Name = "test-cvi"

			cviRules := []v1alpha2.NameReplacement{
				{
					From: v1alpha2.NameReplacementFrom{
						Kind: "ClusterVirtualImage",
						Name: "test-cvi",
					},
					To: "new-cvi-name",
				},
			}

			handler.Override(cviRules)
			Expect(handler.vmbda.Spec.BlockDeviceRef.Name).To(Equal("new-cvi-name"))
		})

		It("should override VirtualImage name in BlockDeviceRef", func() {
			handler.vmbda.Spec.BlockDeviceRef.Kind = v1alpha2.VMBDAObjectRefKindVirtualImage
			handler.vmbda.Spec.BlockDeviceRef.Name = "test-vi"

			viRules := []v1alpha2.NameReplacement{
				{
					From: v1alpha2.NameReplacementFrom{
						Kind: "VirtualImage",
						Name: "test-vi",
					},
					To: "new-vi-name",
				},
			}

			handler.Override(viRules)
			Expect(handler.vmbda.Spec.BlockDeviceRef.Name).To(Equal("new-vi-name"))
		})

		It("should not override non-matching names", func() {
			nonMatchingRules := []v1alpha2.NameReplacement{
				{
					From: v1alpha2.NameReplacementFrom{
						Kind: "VirtualMachineBlockDeviceAttachment",
						Name: "different-vmbda",
					},
					To: "should-not-apply",
				},
			}

			originalName := handler.vmbda.Name
			handler.Override(nonMatchingRules)
			Expect(handler.vmbda.Name).To(Equal(originalName))
		})
	})
})
