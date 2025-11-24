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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VirtualMachineTestArgs struct {
	vmExists               bool
	vmHasCorrectRestoreUID bool
	vmHasCorrectSpec       bool
	hasVMBDAs              bool
	vmbdasHaveCorrectUID   bool

	failValidation bool
	failProcess    bool

	shouldCreateVM     bool
	shouldUpdateVM     bool
	shouldDeleteVMBDAs bool
}

var _ = Describe("VirtualMachineRestorer", func() {
	var (
		ctx context.Context
		err error

		restoreUID string
		vmName     string
		namespace  string

		intercept     interceptor.Funcs
		vmCreated     bool
		vmUpdated     bool
		vmbdasDeleted int

		objects    []client.Object
		vm         v1alpha2.VirtualMachine
		vmbda1     v1alpha2.VirtualMachineBlockDeviceAttachment
		vmbda2     v1alpha2.VirtualMachineBlockDeviceAttachment
		handler    *VirtualMachineHandler
		fakeClient client.WithWatch
	)

	BeforeEach(func() {
		ctx = context.Background()
		restoreUID = "restore-uid-1234"
		vmName = "test-vm"
		namespace = "default"
		vmCreated = false
		vmUpdated = false
		vmbdasDeleted = 0

		objects = []client.Object{}

		vm = v1alpha2.VirtualMachine{
			TypeMeta: metav1.TypeMeta{
				Kind:       "VirtualMachine",
				APIVersion: "virtualization.deckhouse.io/v1alpha2",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      vmName,
				Namespace: namespace,
				Annotations: map[string]string{
					"test-annotation": "test-value",
				},
			},
			Spec: v1alpha2.VirtualMachineSpec{
				RunPolicy:               v1alpha2.AlwaysOnPolicy,
				VirtualMachineIPAddress: "test-ip",
				BlockDeviceRefs: []v1alpha2.BlockDeviceSpecRef{
					{
						Kind: v1alpha2.DiskDevice,
						Name: "test-disk-1",
					},
					{
						Kind: v1alpha2.DiskDevice,
						Name: "test-disk-2",
					},
				},
			},
			Status: v1alpha2.VirtualMachineStatus{
				Conditions: []metav1.Condition{
					{
						Type:   "Maintenance",
						Status: metav1.ConditionTrue,
						Reason: "InMaintenance",
					},
				},
			},
		}

		vmbda1 = v1alpha2.VirtualMachineBlockDeviceAttachment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vmbda-1",
				Namespace: namespace,
				Annotations: map[string]string{
					"other-annotation": "other-value",
				},
			},
			Spec: v1alpha2.VirtualMachineBlockDeviceAttachmentSpec{
				VirtualMachineName: vmName,
				BlockDeviceRef: v1alpha2.VMBDAObjectRef{
					Kind: v1alpha2.VMBDAObjectRefKindVirtualDisk,
					Name: "old-disk-1",
				},
			},
		}

		vmbda2 = v1alpha2.VirtualMachineBlockDeviceAttachment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vmbda-2",
				Namespace: namespace,
				Annotations: map[string]string{
					annotations.AnnVMOPRestore: "different-restore-uid",
				},
			},
			Spec: v1alpha2.VirtualMachineBlockDeviceAttachmentSpec{
				VirtualMachineName: vmName,
				BlockDeviceRef: v1alpha2.VMBDAObjectRef{
					Kind: v1alpha2.VMBDAObjectRefKindVirtualDisk,
					Name: "old-disk-2",
				},
			},
		}

		intercept = interceptor.Funcs{
			Create: func(_ context.Context, _ client.WithWatch, obj client.Object, _ ...client.CreateOption) error {
				if obj.GetName() == vm.Name && obj.GetNamespace() == vm.Namespace {
					_, ok := obj.(*v1alpha2.VirtualMachine)
					Expect(ok).To(BeTrue())
					vmCreated = true
				}
				return nil
			},
			Update: func(_ context.Context, _ client.WithWatch, obj client.Object, _ ...client.UpdateOption) error {
				if obj.GetName() == vm.Name && obj.GetNamespace() == vm.Namespace {
					_, ok := obj.(*v1alpha2.VirtualMachine)
					Expect(ok).To(BeTrue())
					vmUpdated = true
				}
				return nil
			},
			Delete: func(_ context.Context, _ client.WithWatch, obj client.Object, _ ...client.DeleteOption) error {
				if _, ok := obj.(*v1alpha2.VirtualMachineBlockDeviceAttachment); ok {
					vmbdasDeleted++
				}
				return nil
			},
		}
	})

	DescribeTable("restore",
		func(args VirtualMachineTestArgs) {
			if args.vmExists {
				vmToAdd := vm.DeepCopy()
				if args.vmHasCorrectRestoreUID {
					if vmToAdd.Annotations == nil {
						vmToAdd.Annotations = make(map[string]string)
					}
					vmToAdd.Annotations[annotations.AnnVMOPRestore] = restoreUID
				}
				if !args.vmHasCorrectSpec {
					vmToAdd.Spec.VirtualMachineIPAddress = "different-ip"
				}
				objects = append(objects, vmToAdd)
			}

			if args.hasVMBDAs {
				vmbda1Copy := vmbda1.DeepCopy()
				vmbda2Copy := vmbda2.DeepCopy()

				if args.vmbdasHaveCorrectUID {
					if vmbda1Copy.Annotations == nil {
						vmbda1Copy.Annotations = make(map[string]string)
					}
					vmbda1Copy.Annotations[annotations.AnnVMOPRestore] = restoreUID

					if vmbda2Copy.Annotations == nil {
						vmbda2Copy.Annotations = make(map[string]string)
					}
					vmbda2Copy.Annotations[annotations.AnnVMOPRestore] = restoreUID
				}

				objects = append(objects, vmbda1Copy, vmbda2Copy)
			}

			fakeClient, err = testutil.NewFakeClientWithInterceptorWithObjects(intercept, objects...)
			Expect(err).ToNot(HaveOccurred())
			Expect(fakeClient).ToNot(BeNil())

			handler = NewVirtualMachineHandler(fakeClient, vm, restoreUID, v1alpha2.SnapshotOperationModeStrict)
			Expect(handler).ToNot(BeNil())

			// Verify that restore annotation was added
			Expect(handler.vm.Annotations[annotations.AnnVMOPRestore]).To(Equal(restoreUID))

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

			Expect(vmCreated).To(Equal(args.shouldCreateVM))
			Expect(vmUpdated).To(Equal(args.shouldUpdateVM))

			if args.shouldDeleteVMBDAs {
				expectedDeletes := 0
				if args.hasVMBDAs {
					if !args.vmbdasHaveCorrectUID {
						expectedDeletes = 2
					} else {
						expectedDeletes = 0
					}
				}
				Expect(vmbdasDeleted).To(Equal(expectedDeletes))
			}
		},
		Entry("VM doesn't exist", VirtualMachineTestArgs{
			vmExists: false,

			failValidation: false,
			failProcess:    false,

			shouldCreateVM:     true,
			shouldUpdateVM:     false,
			shouldDeleteVMBDAs: true,
		}),
		Entry("VM exists with correct restore UID and correct spec", VirtualMachineTestArgs{
			vmExists:               true,
			vmHasCorrectRestoreUID: true,
			vmHasCorrectSpec:       true,

			failValidation: false,
			failProcess:    false,

			shouldCreateVM:     false,
			shouldUpdateVM:     false,
			shouldDeleteVMBDAs: true,
		}),
		Entry("VM exists with incorrect restore UID", VirtualMachineTestArgs{
			vmExists:               true,
			vmHasCorrectRestoreUID: false,
			vmHasCorrectSpec:       true,

			failValidation: false,
			failProcess:    false,

			shouldCreateVM:     false,
			shouldUpdateVM:     true,
			shouldDeleteVMBDAs: true,
		}),
		Entry("VM exists with incorrect spec", VirtualMachineTestArgs{
			vmExists:               true,
			vmHasCorrectRestoreUID: true,
			vmHasCorrectSpec:       false,

			failValidation: false,
			failProcess:    false,

			shouldCreateVM:     false,
			shouldUpdateVM:     true,
			shouldDeleteVMBDAs: true,
		}),
		Entry("VM exists with incorrect UID and spec", VirtualMachineTestArgs{
			vmExists:               true,
			vmHasCorrectRestoreUID: false,
			vmHasCorrectSpec:       false,

			failValidation: false,
			failProcess:    false,

			shouldCreateVM:     false,
			shouldUpdateVM:     true,
			shouldDeleteVMBDAs: true,
		}),
		Entry("VM exists with VMBDAs that should be deleted", VirtualMachineTestArgs{
			vmExists:               true,
			vmHasCorrectRestoreUID: true,
			vmHasCorrectSpec:       true,
			hasVMBDAs:              true,
			vmbdasHaveCorrectUID:   false,

			failValidation: false,
			failProcess:    false,

			shouldCreateVM:     false,
			shouldUpdateVM:     false,
			shouldDeleteVMBDAs: false,
		}),
		Entry("VM exists with VMBDAs that should not be deleted", VirtualMachineTestArgs{
			vmExists:               true,
			vmHasCorrectRestoreUID: true,
			vmHasCorrectSpec:       true,
			hasVMBDAs:              true,
			vmbdasHaveCorrectUID:   true,

			failValidation: false,
			failProcess:    false,

			shouldCreateVM:     false,
			shouldUpdateVM:     false,
			shouldDeleteVMBDAs: true,
		}),
	)

	Describe("Override", func() {
		var rules []v1alpha2.NameReplacement

		BeforeEach(func() {
			rules = []v1alpha2.NameReplacement{
				{
					From: v1alpha2.NameReplacementFrom{
						Kind: "VirtualMachine",
						Name: vmName,
					},
					To: "new-vm-name",
				},
				{
					From: v1alpha2.NameReplacementFrom{
						Kind: "VirtualMachineIPAddress",
						Name: "test-ip",
					},
					To: "new-test-ip",
				},
				{
					From: v1alpha2.NameReplacementFrom{
						Kind: "VirtualDisk",
						Name: "test-disk-1",
					},
					To: "new-test-disk-1",
				},
				{
					From: v1alpha2.NameReplacementFrom{
						Kind: "Secret",
						Name: "test-secret",
					},
					To: "new-test-secret",
				},
			}

			fakeClient, err = testutil.NewFakeClientWithInterceptorWithObjects(intercept)
			Expect(err).ToNot(HaveOccurred())

			handler = NewVirtualMachineHandler(fakeClient, vm, restoreUID, v1alpha2.SnapshotOperationModeStrict)
		})

		It("should override VM name", func() {
			handler.Override(rules)
			Expect(handler.vm.Name).To(Equal("new-vm-name"))
		})

		It("should override VirtualMachineIPAddress", func() {
			handler.Override(rules)
			Expect(handler.vm.Spec.VirtualMachineIPAddress).To(Equal("new-test-ip"))
		})

		It("should override disk names in BlockDeviceRefs", func() {
			handler.Override(rules)
			Expect(handler.vm.Spec.BlockDeviceRefs[0].Name).To(Equal("new-test-disk-1"))
			Expect(handler.vm.Spec.BlockDeviceRefs[1].Name).To(Equal("test-disk-2")) // unchanged
		})

		It("should override Secret name in UserDataRef", func() {
			handler.vm.Spec.Provisioning = &v1alpha2.Provisioning{
				UserDataRef: &v1alpha2.UserDataRef{
					Kind: v1alpha2.UserDataRefKindSecret,
					Name: "test-secret",
				},
			}
			handler.Override(rules)
			Expect(handler.vm.Spec.Provisioning.UserDataRef.Name).To(Equal("new-test-secret"))
		})

		It("should not override non-matching names", func() {
			nonMatchingRules := []v1alpha2.NameReplacement{
				{
					From: v1alpha2.NameReplacementFrom{
						Kind: "VirtualMachine",
						Name: "different-vm",
					},
					To: "should-not-apply",
				},
			}

			originalName := handler.vm.Name
			handler.Override(nonMatchingRules)
			Expect(handler.vm.Name).To(Equal(originalName))
		})
	})
})
