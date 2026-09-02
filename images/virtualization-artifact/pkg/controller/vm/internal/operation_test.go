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

	Describe("Outcome of the last operation", func() {
		It("Should report a failure with the reason the operation gives", func() {
			vmop := newVMOP("migrate-vm", v1alpha2.VMOPTypeMigrate, v1alpha2.VMOPPhaseFailed)
			vmop.Status.Conditions = []metav1.Condition{
				{
					Type:    vmopcondition.TypeCompleted.String(),
					Status:  metav1.ConditionFalse,
					Reason:  vmopcondition.ReasonNotConverging.String(),
					Message: "Migration cannot converge.",
				},
			}
			fakeClient, resource, vmState = setupEnvironment(newVM(), vmop)

			reconcile()

			cond, found := condition()
			Expect(found).To(BeTrue())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(vmcondition.ReasonOperationFailed.String()))
			Expect(cond.Message).To(ContainSubstring("Migration cannot converge"))
		})

		DescribeTable("Should stay silent about an operation that did not fail", func(phase v1alpha2.VMOPPhase) {
			fakeClient, resource, vmState = setupEnvironment(newVM(), newVMOP("restart-vm", v1alpha2.VMOPTypeRestart, phase))

			reconcile()

			_, found := condition()
			Expect(found).To(BeFalse())
		},
			Entry("completed", v1alpha2.VMOPPhaseCompleted),
			Entry("superseded", v1alpha2.VMOPPhaseSuperseded),
			Entry("terminating", v1alpha2.VMOPPhaseTerminating),
		)

		It("Should clear the failure once a later operation ends without one", func() {
			failed := newVMOP("migrate-vm", v1alpha2.VMOPTypeMigrate, v1alpha2.VMOPPhaseFailed)
			completed := newVMOP("restart-vm", v1alpha2.VMOPTypeRestart, v1alpha2.VMOPPhaseCompleted)
			completed.CreationTimestamp = metav1.NewTime(created.Add(time.Minute))
			fakeClient, resource, vmState = setupEnvironment(newVM(), failed, completed)

			reconcile()

			_, found := condition()
			Expect(found).To(BeFalse())
		})

		It("Should report the failure of the newest operation", func() {
			completed := newVMOP("restart-vm", v1alpha2.VMOPTypeRestart, v1alpha2.VMOPPhaseCompleted)
			failed := newVMOP("start-vm", v1alpha2.VMOPTypeStart, v1alpha2.VMOPPhaseFailed)
			failed.CreationTimestamp = metav1.NewTime(created.Add(time.Minute))
			fakeClient, resource, vmState = setupEnvironment(newVM(), completed, failed)

			reconcile()

			cond, found := condition()
			Expect(found).To(BeTrue())
			Expect(cond.Reason).To(Equal(vmcondition.ReasonOperationFailed.String()))
			Expect(cond.Message).To(ContainSubstring("start-vm"))
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

		It("Should prefer a running snapshot over the failure of a finished operation", func() {
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
})
