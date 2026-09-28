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

package internal

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	commonvmop "github.com/deckhouse/virtualization-controller/pkg/common/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

var _ = Describe("OperationHandler", func() {
	const (
		name      = "vm-operation"
		namespace = "default"
	)

	var (
		ctx        = testutil.ContextBackgroundWithNoOpLogger()
		fakeClient client.WithWatch
		resource   *reconciler.Resource[*v1alpha2.VirtualMachine, v1alpha2.VirtualMachineStatus]
		vmState    state.VirtualMachineState
		created    = metav1.NewTime(time.Now().Add(-time.Hour))
	)

	AfterEach(func() {
		fakeClient = nil
		resource = nil
		vmState = nil
	})

	newVM := func() *v1alpha2.VirtualMachine {
		return vmbuilder.NewEmpty(name, namespace)
	}

	newVMOP := func(vmopName string, vmopType v1alpha2.VMOPType, phase v1alpha2.VMOPPhase) *v1alpha2.VirtualMachineOperation {
		return &v1alpha2.VirtualMachineOperation{
			ObjectMeta: metav1.ObjectMeta{
				Name:              vmopName,
				Namespace:         namespace,
				CreationTimestamp: created,
			},
			Spec: v1alpha2.VirtualMachineOperationSpec{
				Type:           vmopType,
				VirtualMachine: name,
			},
			Status: v1alpha2.VirtualMachineOperationStatus{
				Phase: phase,
			},
		}
	}

	newSnapshot := func(snapshotName string, phase v1alpha2.VirtualMachineSnapshotPhase) *v1alpha2.VirtualMachineSnapshot {
		return &v1alpha2.VirtualMachineSnapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      snapshotName,
				Namespace: namespace,
			},
			Spec: v1alpha2.VirtualMachineSnapshotSpec{
				VirtualMachineName: name,
			},
			Status: v1alpha2.VirtualMachineSnapshotStatus{
				Phase: phase,
			},
		}
	}

	newVMBDA := func(vmbdaName string, phase v1alpha2.BlockDeviceAttachmentPhase) *v1alpha2.VirtualMachineBlockDeviceAttachment {
		return &v1alpha2.VirtualMachineBlockDeviceAttachment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      vmbdaName,
				Namespace: namespace,
			},
			Spec: v1alpha2.VirtualMachineBlockDeviceAttachmentSpec{
				VirtualMachineName: name,
				BlockDeviceRef: v1alpha2.VMBDAObjectRef{
					Kind: v1alpha2.VMBDAObjectRefKindVirtualDisk,
					Name: "data-disk",
				},
			},
			Status: v1alpha2.VirtualMachineBlockDeviceAttachmentStatus{
				Phase: phase,
			},
		}
	}

	reconcile := func() {
		h := NewOperationHandler(fakeClient)
		_, err := h.Handle(ctx, vmState)
		Expect(err).NotTo(HaveOccurred())
		err = resource.Update(context.Background())
		Expect(err).NotTo(HaveOccurred())
	}

	condition := func() (metav1.Condition, bool) {
		updated := &v1alpha2.VirtualMachine{}
		err := fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, updated)
		Expect(err).NotTo(HaveOccurred())
		return conditions.GetCondition(vmcondition.TypeOperationInProgress, updated.Status.Conditions)
	}

	DescribeTable("Should report a running operation", func(vmop *v1alpha2.VirtualMachineOperation, expected vmcondition.OperationInProgressReason) {
		fakeClient, resource, vmState = setupEnvironment(newVM(), vmop)

		reconcile()

		cond, found := condition()
		Expect(found).To(BeTrue())
		Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		Expect(cond.Reason).To(Equal(expected.String()))
		Expect(cond.Message).To(ContainSubstring(vmop.GetName()))
	},
		Entry("start", newVMOP("start-vm", v1alpha2.VMOPTypeStart, v1alpha2.VMOPPhaseInProgress), vmcondition.ReasonVirtualMachineStarting),
		Entry("stop", newVMOP("stop-vm", v1alpha2.VMOPTypeStop, v1alpha2.VMOPPhaseInProgress), vmcondition.ReasonVirtualMachineStopping),
		Entry("restart", newVMOP("restart-vm", v1alpha2.VMOPTypeRestart, v1alpha2.VMOPPhasePending), vmcondition.ReasonVirtualMachineRestarting),
		Entry("migration", newVMOP("migrate-vm", v1alpha2.VMOPTypeMigrate, v1alpha2.VMOPPhaseInProgress), vmcondition.ReasonVirtualMachineMigrating),
		Entry("eviction", newVMOP("evict-vm", v1alpha2.VMOPTypeEvict, v1alpha2.VMOPPhaseInProgress), vmcondition.ReasonVirtualMachineEvacuating),
		Entry("restore", newVMOP("restore-vm", v1alpha2.VMOPTypeRestore, v1alpha2.VMOPPhaseInProgress), vmcondition.ReasonVirtualMachineRestoring),
		Entry("clone", newVMOP("clone-vm", v1alpha2.VMOPTypeClone, v1alpha2.VMOPPhaseInProgress), vmcondition.ReasonVirtualMachineCloning),
	)

	Describe("Evictions created by the controllers of the module", func() {
		It("Should report a volume migration by its annotation", func() {
			vmop := newVMOP("volume-migration-abc", v1alpha2.VMOPTypeEvict, v1alpha2.VMOPPhaseInProgress)
			vmop.Annotations = map[string]string{annotations.AnnVMOPVolumeMigration: "true"}
			fakeClient, resource, vmState = setupEnvironment(newVM(), vmop)

			reconcile()

			cond, found := condition()
			Expect(found).To(BeTrue())
			Expect(cond.Reason).To(Equal(vmcondition.ReasonVolumeMigrating.String()))
		})

		DescribeTable("Should tell workload updates apart by the generated name", func(prefix string, expected vmcondition.OperationInProgressReason) {
			vmop := newVMOP(prefix+"xyz", v1alpha2.VMOPTypeEvict, v1alpha2.VMOPPhaseInProgress)
			vmop.Annotations = map[string]string{annotations.AnnVMOPWorkloadUpdate: "true"}
			fakeClient, resource, vmState = setupEnvironment(newVM(), vmop)

			reconcile()

			cond, found := condition()
			Expect(found).To(BeTrue())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(expected.String()))
		},
			Entry("firmware", commonvmop.FirmwareUpdatePrefix, vmcondition.ReasonFirmwareUpdating),
			Entry("node placement", commonvmop.NodePlacementUpdatePrefix, vmcondition.ReasonNodePlacementUpdating),
			Entry("hot-plugged resources", commonvmop.HotplugResourcesPrefix, vmcondition.ReasonResourcesHotplugging),
			Entry("an update of an unknown kind", "workload-update-", vmcondition.ReasonWorkloadUpdating),
		)
	})

	Describe("A finished operation", func() {
		DescribeTable("Should not be reported whatever its outcome", func(phase v1alpha2.VMOPPhase) {
			fakeClient, resource, vmState = setupEnvironment(newVM(), newVMOP("restart-vm", v1alpha2.VMOPTypeRestart, phase))

			reconcile()

			_, found := condition()
			Expect(found).To(BeFalse())
		},
			Entry("completed", v1alpha2.VMOPPhaseCompleted),
			Entry("failed", v1alpha2.VMOPPhaseFailed),
			Entry("superseded", v1alpha2.VMOPPhaseSuperseded),
			Entry("terminating", v1alpha2.VMOPPhaseTerminating),
		)

		It("Should drop a failure reported before", func() {
			vm := newVM()
			vm.Status.Conditions = []metav1.Condition{
				{
					Type:               vmcondition.TypeOperationInProgress.String(),
					Status:             metav1.ConditionFalse,
					Reason:             "OperationFailed",
					Message:            "Start of the virtual machine has failed; VirtualMachineOperation: start-vm.",
					LastTransitionTime: metav1.Now(),
				},
			}
			fakeClient, resource, vmState = setupEnvironment(vm, newVMOP("start-vm", v1alpha2.VMOPTypeStart, v1alpha2.VMOPPhaseFailed))

			reconcile()

			_, found := condition()
			Expect(found).To(BeFalse())
		})

		It("Should not hide an operation started after a failed one", func() {
			failed := newVMOP("start-vm", v1alpha2.VMOPTypeStart, v1alpha2.VMOPPhaseFailed)
			running := newVMOP("start-vm-again", v1alpha2.VMOPTypeStart, v1alpha2.VMOPPhaseInProgress)
			running.CreationTimestamp = metav1.NewTime(created.Add(time.Minute))
			fakeClient, resource, vmState = setupEnvironment(newVM(), failed, running)

			reconcile()

			cond, found := condition()
			Expect(found).To(BeTrue())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(vmcondition.ReasonVirtualMachineStarting.String()))
			Expect(cond.Message).To(ContainSubstring("start-vm-again"))
		})
	})

	Describe("Operations described by their own resources", func() {
		It("Should report a snapshot being taken", func() {
			fakeClient, resource, vmState = setupEnvironment(newVM(), newSnapshot("vm-snapshot", v1alpha2.VirtualMachineSnapshotPhaseInProgress))

			reconcile()

			cond, found := condition()
			Expect(found).To(BeTrue())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(vmcondition.ReasonVirtualMachineSnapshotting.String()))
			Expect(cond.Message).To(ContainSubstring("vm-snapshot"))
		})

		It("Should report a block device being attached", func() {
			fakeClient, resource, vmState = setupEnvironment(newVM(), newVMBDA("attach-data-disk", v1alpha2.BlockDeviceAttachmentPhaseInProgress))

			reconcile()

			cond, found := condition()
			Expect(found).To(BeTrue())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(vmcondition.ReasonBlockDeviceAttaching.String()))
			Expect(cond.Message).To(ContainSubstring("data-disk"))
		})

		It("Should report a block device being detached", func() {
			fakeClient, resource, vmState = setupEnvironment(newVM(), newVMBDA("attach-data-disk", v1alpha2.BlockDeviceAttachmentPhaseTerminating))

			reconcile()

			cond, found := condition()
			Expect(found).To(BeTrue())
			Expect(cond.Reason).To(Equal(vmcondition.ReasonBlockDeviceDetaching.String()))
		})

		It("Should not report an attached block device", func() {
			fakeClient, resource, vmState = setupEnvironment(newVM(), newVMBDA("attach-data-disk", v1alpha2.BlockDeviceAttachmentPhaseAttached))

			reconcile()

			_, found := condition()
			Expect(found).To(BeFalse())
		})
	})

	Describe("A power state change nobody asked for", func() {
		It("Should report a restart requested by the controller itself", func() {
			kvvm := newEmptyKVVM(name, namespace)
			kvvm.Status.StateChangeRequests = []virtv1.VirtualMachineStateChangeRequest{
				{Action: virtv1.StopRequest},
				{Action: virtv1.StartRequest},
			}
			fakeClient, resource, vmState = setupEnvironment(newVM(), kvvm)

			reconcile()

			cond, found := condition()
			Expect(found).To(BeTrue())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(vmcondition.ReasonVirtualMachineRestarting.String()))
		})

		It("Should not report a machine with no pending requests", func() {
			fakeClient, resource, vmState = setupEnvironment(newVM(), newEmptyKVVM(name, namespace))

			reconcile()

			_, found := condition()
			Expect(found).To(BeFalse())
		})
	})

	Describe("Order of importance", func() {
		It("Should prefer a running operation over a snapshot", func() {
			fakeClient, resource, vmState = setupEnvironment(
				newVM(),
				newVMOP("migrate-vm", v1alpha2.VMOPTypeMigrate, v1alpha2.VMOPPhaseInProgress),
				newSnapshot("vm-snapshot", v1alpha2.VirtualMachineSnapshotPhaseInProgress),
			)

			reconcile()

			cond, _ := condition()
			Expect(cond.Reason).To(Equal(vmcondition.ReasonVirtualMachineMigrating.String()))
		})

		It("Should report a running snapshot next to a failed operation", func() {
			fakeClient, resource, vmState = setupEnvironment(
				newVM(),
				newVMOP("migrate-vm", v1alpha2.VMOPTypeMigrate, v1alpha2.VMOPPhaseFailed),
				newSnapshot("vm-snapshot", v1alpha2.VirtualMachineSnapshotPhaseInProgress),
			)

			reconcile()

			cond, _ := condition()
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(vmcondition.ReasonVirtualMachineSnapshotting.String()))
		})

		It("Should remove the condition when nothing is known about the machine", func() {
			vm := newVM()
			vm.Status.Conditions = []metav1.Condition{
				{
					Type:               vmcondition.TypeOperationInProgress.String(),
					Status:             metav1.ConditionTrue,
					Reason:             vmcondition.ReasonVirtualMachineMigrating.String(),
					Message:            "stale",
					LastTransitionTime: metav1.Now(),
				},
			}
			fakeClient, resource, vmState = setupEnvironment(vm)

			reconcile()

			_, found := condition()
			Expect(found).To(BeFalse())
		})
	})

	Describe("Details of a running operation", func() {
		It("Should add the current step the operation reports", func() {
			vmop := newVMOP("migrate-vm", v1alpha2.VMOPTypeMigrate, v1alpha2.VMOPPhaseInProgress)
			vmop.Status.Conditions = []metav1.Condition{
				{
					Type:    vmopcondition.TypeCompleted.String(),
					Status:  metav1.ConditionFalse,
					Reason:  vmopcondition.ReasonTargetScheduling.String(),
					Message: "The target pod is being scheduled.",
				},
			}
			fakeClient, resource, vmState = setupEnvironment(newVM(), vmop)

			reconcile()

			cond, _ := condition()
			Expect(cond.Message).To(Equal("The virtual machine is being migrated to another node: The target pod is being scheduled; VirtualMachineOperation: migrate-vm."))
		})

		It("Should report an operation that has no phase yet", func() {
			fakeClient, resource, vmState = setupEnvironment(newVM(), newVMOP("restart-vm", v1alpha2.VMOPTypeRestart, ""))

			reconcile()

			cond, found := condition()
			Expect(found).To(BeTrue())
			Expect(cond.Reason).To(Equal(vmcondition.ReasonVirtualMachineRestarting.String()))
		})

		It("Should report the oldest of several running operations", func() {
			older := newVMOP("stop-vm", v1alpha2.VMOPTypeStop, v1alpha2.VMOPPhaseInProgress)
			newer := newVMOP("migrate-vm", v1alpha2.VMOPTypeMigrate, v1alpha2.VMOPPhasePending)
			newer.CreationTimestamp = metav1.NewTime(created.Add(time.Minute))
			fakeClient, resource, vmState = setupEnvironment(newVM(), newer, older)

			reconcile()

			cond, _ := condition()
			Expect(cond.Reason).To(Equal(vmcondition.ReasonVirtualMachineStopping.String()))
			Expect(cond.Message).To(ContainSubstring("stop-vm"))
		})

		It("Should skip an operation of a type it does not describe", func() {
			fakeClient, resource, vmState = setupEnvironment(
				newVM(),
				newVMOP("unknown-vm", v1alpha2.VMOPType("Unknown"), v1alpha2.VMOPPhaseInProgress),
				newSnapshot("vm-snapshot", v1alpha2.VirtualMachineSnapshotPhaseInProgress),
			)

			reconcile()

			cond, _ := condition()
			Expect(cond.Reason).To(Equal(vmcondition.ReasonVirtualMachineSnapshotting.String()))
		})
	})

	Describe("Snapshots of the machine", func() {
		It("Should tell a snapshot waiting for its turn apart from one being taken", func() {
			fakeClient, resource, vmState = setupEnvironment(newVM(), newSnapshot("vm-snapshot", v1alpha2.VirtualMachineSnapshotPhasePending))

			reconcile()

			cond, _ := condition()
			Expect(cond.Reason).To(Equal(vmcondition.ReasonVirtualMachineSnapshotting.String()))
			Expect(cond.Message).To(Equal("The virtual machine is selected for taking a snapshot; VirtualMachineSnapshot: vm-snapshot."))
		})

		It("Should not report a snapshot being deleted", func() {
			snapshot := newSnapshot("vm-snapshot", v1alpha2.VirtualMachineSnapshotPhaseInProgress)
			snapshot.Finalizers = []string{"test"}
			snapshot.DeletionTimestamp = ptr.To(metav1.Now())
			fakeClient, resource, vmState = setupEnvironment(newVM(), snapshot)

			reconcile()

			_, found := condition()
			Expect(found).To(BeFalse())
		})

		It("Should not report a snapshot that is ready", func() {
			fakeClient, resource, vmState = setupEnvironment(newVM(), newSnapshot("vm-snapshot", v1alpha2.VirtualMachineSnapshotPhaseReady))

			reconcile()

			_, found := condition()
			Expect(found).To(BeFalse())
		})

		It("Should not report a snapshot of another machine", func() {
			snapshot := newSnapshot("other-snapshot", v1alpha2.VirtualMachineSnapshotPhaseInProgress)
			snapshot.Spec.VirtualMachineName = "other-vm"
			fakeClient, resource, vmState = setupEnvironment(newVM(), snapshot)

			reconcile()

			_, found := condition()
			Expect(found).To(BeFalse())
		})
	})

	Describe("Block devices of the machine", func() {
		It("Should report a block device waiting to be attached", func() {
			fakeClient, resource, vmState = setupEnvironment(newVM(), newVMBDA("attach-data-disk", v1alpha2.BlockDeviceAttachmentPhasePending))

			reconcile()

			cond, _ := condition()
			Expect(cond.Reason).To(Equal(vmcondition.ReasonBlockDeviceAttaching.String()))
			Expect(cond.Message).To(Equal(`The VirtualDisk "data-disk" is being attached to the virtual machine; VirtualMachineBlockDeviceAttachment: attach-data-disk.`))
		})

		It("Should report an attachment being deleted as a detach", func() {
			vmbda := newVMBDA("attach-data-disk", v1alpha2.BlockDeviceAttachmentPhaseAttached)
			vmbda.Finalizers = []string{"test"}
			vmbda.DeletionTimestamp = ptr.To(metav1.Now())
			fakeClient, resource, vmState = setupEnvironment(newVM(), vmbda)

			reconcile()

			cond, _ := condition()
			Expect(cond.Reason).To(Equal(vmcondition.ReasonBlockDeviceDetaching.String()))
		})

		It("Should prefer a detach over an attach", func() {
			fakeClient, resource, vmState = setupEnvironment(
				newVM(),
				newVMBDA("attach-new-disk", v1alpha2.BlockDeviceAttachmentPhaseInProgress),
				newVMBDA("attach-old-disk", v1alpha2.BlockDeviceAttachmentPhaseTerminating),
			)

			reconcile()

			cond, _ := condition()
			Expect(cond.Reason).To(Equal(vmcondition.ReasonBlockDeviceDetaching.String()))
			Expect(cond.Message).To(ContainSubstring("attach-old-disk"))
		})
	})

	DescribeTable("Should report a power state change requested by the controller", func(actions []virtv1.StateChangeRequestAction, expected vmcondition.OperationInProgressReason) {
		kvvm := newEmptyKVVM(name, namespace)
		for _, action := range actions {
			kvvm.Status.StateChangeRequests = append(kvvm.Status.StateChangeRequests, virtv1.VirtualMachineStateChangeRequest{Action: action})
		}
		fakeClient, resource, vmState = setupEnvironment(newVM(), kvvm)

		reconcile()

		cond, found := condition()
		Expect(found).To(BeTrue())
		Expect(cond.Reason).To(Equal(expected.String()))
		Expect(cond.Message).NotTo(ContainSubstring("VirtualMachineOperation"))
	},
		Entry("start", []virtv1.StateChangeRequestAction{virtv1.StartRequest}, vmcondition.ReasonVirtualMachineStarting),
		Entry("stop", []virtv1.StateChangeRequestAction{virtv1.StopRequest}, vmcondition.ReasonVirtualMachineStopping),
	)

	Describe("More order of importance", func() {
		It("Should prefer a snapshot over a block device", func() {
			fakeClient, resource, vmState = setupEnvironment(
				newVM(),
				newSnapshot("vm-snapshot", v1alpha2.VirtualMachineSnapshotPhaseInProgress),
				newVMBDA("attach-data-disk", v1alpha2.BlockDeviceAttachmentPhaseInProgress),
			)

			reconcile()

			cond, _ := condition()
			Expect(cond.Reason).To(Equal(vmcondition.ReasonVirtualMachineSnapshotting.String()))
		})

		It("Should prefer a block device over a power state change requested by the controller", func() {
			kvvm := newEmptyKVVM(name, namespace)
			kvvm.Status.StateChangeRequests = []virtv1.VirtualMachineStateChangeRequest{{Action: virtv1.StopRequest}}
			fakeClient, resource, vmState = setupEnvironment(newVM(), kvvm, newVMBDA("attach-data-disk", v1alpha2.BlockDeviceAttachmentPhaseInProgress))

			reconcile()

			cond, _ := condition()
			Expect(cond.Reason).To(Equal(vmcondition.ReasonBlockDeviceAttaching.String()))
		})
	})

	Describe("The machine itself", func() {
		It("Should set the generation of the machine it has observed", func() {
			vm := newVM()
			vm.Generation = 7
			fakeClient, resource, vmState = setupEnvironment(vm, newVMOP("restart-vm", v1alpha2.VMOPTypeRestart, v1alpha2.VMOPPhaseInProgress))

			reconcile()

			cond, _ := condition()
			Expect(cond.ObservedGeneration).To(Equal(int64(7)))
		})

		It("Should leave the condition of a machine being deleted as it is", func() {
			vm := newVM()
			vm.Finalizers = []string{"test"}
			vm.DeletionTimestamp = ptr.To(metav1.Now())
			vm.Status.Conditions = []metav1.Condition{
				{
					Type:               vmcondition.TypeOperationInProgress.String(),
					Status:             metav1.ConditionTrue,
					Reason:             vmcondition.ReasonVirtualMachineStopping.String(),
					Message:            "The virtual machine is stopping.",
					LastTransitionTime: metav1.Now(),
				},
			}
			fakeClient, resource, vmState = setupEnvironment(vm)

			reconcile()

			cond, found := condition()
			Expect(found).To(BeTrue())
			Expect(cond.Reason).To(Equal(vmcondition.ReasonVirtualMachineStopping.String()))
		})
	})
})
