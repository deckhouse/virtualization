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

package internal

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmbdacondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

var _ = Describe("DeletionHandler", func() {
	It("sets Terminating condition while waiting for block device detach", func() {
		now := metav1.Now()
		vmbda := &v1alpha2.VirtualMachineBlockDeviceAttachment{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "vmbda",
				Namespace:         "default",
				DeletionTimestamp: &now,
			},
			Spec: v1alpha2.VirtualMachineBlockDeviceAttachmentSpec{
				VirtualMachineName: "vm",
				BlockDeviceRef: v1alpha2.VMBDAObjectRef{
					Kind: v1alpha2.VMBDAObjectRefKindVirtualDisk,
					Name: "vd",
				},
			},
		}
		vm := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vm",
				Namespace: "default",
			},
		}
		kvvm := &virtv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vm",
				Namespace: "default",
			},
		}

		client, err := testutil.NewFakeClientWithObjects(vm, kvvm)
		Expect(err).NotTo(HaveOccurred())
		handler := NewDeletionHandler(vmbdaUnplug{
			isAttached: func(*v1alpha2.VirtualMachine, *virtv1.VirtualMachine, *v1alpha2.VirtualMachineBlockDeviceAttachment) bool {
				return true
			},
			unplugDisk: func(context.Context, *virtv1.VirtualMachine, string) error {
				return nil
			},
		}, client)

		result, err := handler.Handle(context.Background(), vmbda)

		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeZero())
		Expect(vmbda.Finalizers).To(ContainElement(v1alpha2.FinalizerVMBDACleanup))

		cond, ok := conditions.GetCondition(vmbdacondition.TerminatingType, vmbda.Status.Conditions)
		Expect(ok).To(BeTrue())
		Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		Expect(cond.Reason).To(Equal(vmbdacondition.DetachPending.String()))
		Expect(cond.Message).To(Equal(`Waiting for the VirtualDisk "vd" to detach from the VirtualMachine "vm".`))
	})

	It("waits for migration to finish instead of unplugging while the VM is migrating", func() {
		now := metav1.Now()
		vmbda := &v1alpha2.VirtualMachineBlockDeviceAttachment{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "vmbda",
				Namespace:         "default",
				DeletionTimestamp: &now,
			},
			Spec: v1alpha2.VirtualMachineBlockDeviceAttachmentSpec{
				VirtualMachineName: "vm",
				BlockDeviceRef: v1alpha2.VMBDAObjectRef{
					Kind: v1alpha2.VMBDAObjectRefKindVirtualDisk,
					Name: "vd",
				},
			},
		}
		vm := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vm",
				Namespace: "default",
			},
			Status: v1alpha2.VirtualMachineStatus{
				Conditions: []metav1.Condition{
					{
						Type:   vmcondition.TypeMigrating.String(),
						Status: metav1.ConditionTrue,
					},
				},
			},
		}
		kvvm := &virtv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vm",
				Namespace: "default",
			},
		}

		client, err := testutil.NewFakeClientWithObjects(vm, kvvm)
		Expect(err).NotTo(HaveOccurred())
		unplugCalled := false
		handler := NewDeletionHandler(vmbdaUnplug{
			isAttached: func(*v1alpha2.VirtualMachine, *virtv1.VirtualMachine, *v1alpha2.VirtualMachineBlockDeviceAttachment) bool {
				return true
			},
			unplugDisk: func(context.Context, *virtv1.VirtualMachine, string) error {
				unplugCalled = true
				return nil
			},
		}, client)

		result, err := handler.Handle(context.Background(), vmbda)

		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))
		Expect(unplugCalled).To(BeFalse())
		Expect(vmbda.Finalizers).To(ContainElement(v1alpha2.FinalizerVMBDACleanup))

		cond, ok := conditions.GetCondition(vmbdacondition.TerminatingType, vmbda.Status.Conditions)
		Expect(ok).To(BeTrue())
		Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		Expect(cond.Reason).To(Equal(vmbdacondition.TerminatingBlockedByMigration.String()))
		Expect(cond.Message).To(Equal(`Cannot detach the VirtualDisk "vd" from the VirtualMachine "vm" while it is migrating.`))
	})

	DescribeTable("decides on detach by the state of the migrating operation", func(reason vmopcondition.ReasonCompleted, expectBlocked bool) {
		now := metav1.Now()
		vmbda := &v1alpha2.VirtualMachineBlockDeviceAttachment{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "vmbda",
				Namespace:         "default",
				DeletionTimestamp: &now,
			},
			Spec: v1alpha2.VirtualMachineBlockDeviceAttachmentSpec{
				VirtualMachineName: "vm",
				BlockDeviceRef: v1alpha2.VMBDAObjectRef{
					Kind: v1alpha2.VMBDAObjectRefKindVirtualDisk,
					Name: "vd",
				},
			},
		}
		vm := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "default"},
			Status: v1alpha2.VirtualMachineStatus{
				Conditions: []metav1.Condition{
					{
						Type:   vmcondition.TypeMigrating.String(),
						Status: metav1.ConditionFalse,
						Reason: vmcondition.ReasonMigratingPending.String(),
					},
				},
			},
		}
		kvvm := &virtv1.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "default"}}
		vmop := &v1alpha2.VirtualMachineOperation{
			ObjectMeta: metav1.ObjectMeta{Name: "vmop", Namespace: "default"},
			Spec: v1alpha2.VirtualMachineOperationSpec{
				Type:           v1alpha2.VMOPTypeMigrate,
				VirtualMachine: "vm",
			},
			Status: v1alpha2.VirtualMachineOperationStatus{
				Phase: v1alpha2.VMOPPhasePending,
				Conditions: []metav1.Condition{
					{
						Type:   vmopcondition.TypeCompleted.String(),
						Status: metav1.ConditionFalse,
						Reason: reason.String(),
					},
				},
			},
		}

		client, err := testutil.NewFakeClientWithObjects(vm, kvvm, vmop)
		Expect(err).NotTo(HaveOccurred())
		unplugCalled := false
		handler := NewDeletionHandler(vmbdaUnplug{
			isAttached: func(*v1alpha2.VirtualMachine, *virtv1.VirtualMachine, *v1alpha2.VirtualMachineBlockDeviceAttachment) bool {
				return true
			},
			unplugDisk: func(context.Context, *virtv1.VirtualMachine, string) error {
				unplugCalled = true
				return nil
			},
		}, client)

		_, err = handler.Handle(context.Background(), vmbda)
		Expect(err).NotTo(HaveOccurred())
		Expect(unplugCalled).To(Equal(!expectBlocked))

		cond, ok := conditions.GetCondition(vmbdacondition.TerminatingType, vmbda.Status.Conditions)
		Expect(ok).To(BeTrue())
		if expectBlocked {
			Expect(cond.Reason).To(Equal(vmbdacondition.TerminatingBlockedByMigration.String()))
		} else {
			Expect(cond.Reason).To(Equal(vmbdacondition.DetachPending.String()))
		}
	},
		Entry("parked on the project quota", vmopcondition.ReasonQuotaExceeded, false),
		Entry("waiting for another hot-plug request", vmopcondition.ReasonWaitingForBlockDeviceAttachment, false),
		Entry("target is being scheduled", vmopcondition.ReasonTargetScheduling, true),
		Entry("target is being prepared", vmopcondition.ReasonTargetPreparing, true),
		Entry("waiting for a sync slot on a prepared target", vmopcondition.ReasonWaitingForSyncSlot, true),
		Entry("queued behind an inbound migration slot", vmopcondition.ReasonMigrationPending, true),
		Entry("migration is running", vmopcondition.ReasonMigrationRunning, true),
	)

	It("keeps the finalizer while the volume is still plugged into the running machine", func() {
		now := metav1.Now()
		vmbda := &v1alpha2.VirtualMachineBlockDeviceAttachment{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "vmbda",
				Namespace:         "default",
				DeletionTimestamp: &now,
				Finalizers:        []string{v1alpha2.FinalizerVMBDACleanup},
			},
			Spec: v1alpha2.VirtualMachineBlockDeviceAttachmentSpec{
				VirtualMachineName: "vm",
				BlockDeviceRef: v1alpha2.VMBDAObjectRef{
					Kind: v1alpha2.VMBDAObjectRefKindVirtualDisk,
					Name: "vd",
				},
			},
		}
		vm := &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "default"}}
		kvvm := &virtv1.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "default"}}
		kvvmi := &virtv1.VirtualMachineInstance{
			ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "default"},
			Spec: virtv1.VirtualMachineInstanceSpec{
				Volumes: []virtv1.Volume{{Name: kvbuilder.GenerateVMBDADiskName(vmbda.Spec.BlockDeviceRef)}},
			},
		}

		client, err := testutil.NewFakeClientWithObjects(vm, kvvm, kvvmi)
		Expect(err).NotTo(HaveOccurred())

		unplugCalled := false
		handler := NewDeletionHandler(vmbdaUnplug{
			// The block device references of the VirtualMachine no longer mention the attachment.
			isAttached: func(*v1alpha2.VirtualMachine, *virtv1.VirtualMachine, *v1alpha2.VirtualMachineBlockDeviceAttachment) bool {
				return false
			},
			unplugDisk: func(context.Context, *virtv1.VirtualMachine, string) error {
				unplugCalled = true
				return nil
			},
		}, client)

		_, err = handler.Handle(context.Background(), vmbda)
		Expect(err).NotTo(HaveOccurred())

		Expect(vmbda.Finalizers).To(ContainElement(v1alpha2.FinalizerVMBDACleanup))
		Expect(unplugCalled).To(BeTrue())

		cond, ok := conditions.GetCondition(vmbdacondition.TerminatingType, vmbda.Status.Conditions)
		Expect(ok).To(BeTrue())
		Expect(cond.Reason).To(Equal(vmbdacondition.DetachPending.String()))
	})

	It("removes the cleanup finalizer when the VirtualMachine is gone and nothing is attached", func() {
		now := metav1.Now()
		vmbda := &v1alpha2.VirtualMachineBlockDeviceAttachment{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "vmbda",
				Namespace:         "default",
				DeletionTimestamp: &now,
				Finalizers:        []string{v1alpha2.FinalizerVMBDACleanup},
			},
			Spec: v1alpha2.VirtualMachineBlockDeviceAttachmentSpec{
				VirtualMachineName: "vm",
				BlockDeviceRef: v1alpha2.VMBDAObjectRef{
					Kind: v1alpha2.VMBDAObjectRefKindVirtualDisk,
					Name: "vd",
				},
			},
		}

		var gotVM *v1alpha2.VirtualMachine
		var gotKVVM *virtv1.VirtualMachine

		// No VM and no internal VirtualMachine exist in the cluster: FetchObject returns nil
		// for both, and IsAttached is still consulted with those nils.
		client, err := testutil.NewFakeClientWithObjects()
		Expect(err).NotTo(HaveOccurred())
		handler := NewDeletionHandler(vmbdaUnplug{
			isAttached: func(vm *v1alpha2.VirtualMachine, kvvm *virtv1.VirtualMachine, _ *v1alpha2.VirtualMachineBlockDeviceAttachment) bool {
				gotVM, gotKVVM = vm, kvvm
				return false
			},
			unplugDisk: func(context.Context, *virtv1.VirtualMachine, string) error {
				return nil
			},
		}, client)

		result, err := handler.Handle(context.Background(), vmbda)

		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeZero())
		Expect(gotVM).To(BeNil())
		Expect(gotKVVM).To(BeNil())
		Expect(vmbda.Finalizers).NotTo(ContainElement(v1alpha2.FinalizerVMBDACleanup))

		_, ok := conditions.GetCondition(vmbdacondition.TerminatingType, vmbda.Status.Conditions)
		Expect(ok).To(BeFalse())
	})
})

type vmbdaUnplug struct {
	isAttached func(*v1alpha2.VirtualMachine, *virtv1.VirtualMachine, *v1alpha2.VirtualMachineBlockDeviceAttachment) bool
	unplugDisk func(context.Context, *virtv1.VirtualMachine, string) error
}

func (u vmbdaUnplug) IsAttached(vm *v1alpha2.VirtualMachine, kvvm *virtv1.VirtualMachine, vmbda *v1alpha2.VirtualMachineBlockDeviceAttachment) bool {
	return u.isAttached(vm, kvvm, vmbda)
}

func (u vmbdaUnplug) UnplugDisk(ctx context.Context, kvvm *virtv1.VirtualMachine, diskName string) error {
	return u.unplugDisk(ctx, kvvm, diskName)
}
