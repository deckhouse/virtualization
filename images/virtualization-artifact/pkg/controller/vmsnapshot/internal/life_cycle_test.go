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

package internal

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmscondition"
)

var _ = Describe("LifeCycle handler", func() {
	var recorder eventrecord.EventRecorderLogger
	var snapshotter *SnapshotterMock
	var storer *StorerMock
	var vd *v1alpha2.VirtualDisk
	var vm *v1alpha2.VirtualMachine
	var secret *corev1.Secret
	var vdSnapshot *v1alpha2.VirtualDiskSnapshot
	var vmSnapshot *v1alpha2.VirtualMachineSnapshot
	var fakeClient client.WithWatch

	BeforeEach(func() {
		vd = &v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{Name: "vd-bar"},
			Status: v1alpha2.VirtualDiskStatus{
				Phase: v1alpha2.DiskReady,
				Conditions: []metav1.Condition{
					{
						Type:   vdcondition.Ready.String(),
						Status: metav1.ConditionTrue,
					},
				},
			},
		}

		vm = &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "vm"},
			Spec: v1alpha2.VirtualMachineSpec{
				BlockDeviceRefs: []v1alpha2.BlockDeviceSpecRef{
					{
						Kind: v1alpha2.DiskDevice,
						Name: vd.Name,
					},
				},
			},
			Status: v1alpha2.VirtualMachineStatus{
				Phase: v1alpha2.MachineRunning,
				BlockDeviceRefs: []v1alpha2.BlockDeviceStatusRef{
					{
						Kind: v1alpha2.DiskDevice,
						Name: vd.Name,
					},
				},
				Conditions: []metav1.Condition{
					{
						Type:   vmcondition.TypeBlockDevicesReady.String(),
						Status: metav1.ConditionTrue,
					},
				},
			},
		}

		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: vm.Name},
		}

		vmSnapshot = &v1alpha2.VirtualMachineSnapshot{
			ObjectMeta: metav1.ObjectMeta{Name: "vm-snapshot"},
			Spec: v1alpha2.VirtualMachineSnapshotSpec{
				VirtualMachineName:  vm.Name,
				RequiredConsistency: true,
			},
			Status: v1alpha2.VirtualMachineSnapshotStatus{
				VirtualMachineSnapshotSecretName: "vm-snapshot",
				Conditions: []metav1.Condition{
					{
						Type:   vmscondition.VirtualMachineReadyType.String(),
						Status: metav1.ConditionTrue,
					},
				},
			},
		}

		vdSnapshot = &v1alpha2.VirtualDiskSnapshot{
			ObjectMeta: metav1.ObjectMeta{Name: getVDSnapshotName(vd.Name, vmSnapshot)},
			Status: v1alpha2.VirtualDiskSnapshotStatus{
				Phase:      v1alpha2.VirtualDiskSnapshotPhaseReady,
				Consistent: ptr.To(true),
			},
		}

		snapshotter = &SnapshotterMock{
			GetVirtualDiskFunc: func(_ context.Context, name, namespace string) (*v1alpha2.VirtualDisk, error) {
				return vd, nil
			},
			GetVirtualMachineFunc: func(_ context.Context, _, _ string) (*v1alpha2.VirtualMachine, error) {
				return vm, nil
			},
			IsFrozenFunc: func(_ context.Context, _ *v1alpha2.VirtualMachine) (bool, error) {
				return true, nil
			},
			CanUnfreezeWithVirtualMachineSnapshotFunc: func(_ context.Context, _ string, _ *v1alpha2.VirtualMachine) (bool, error) {
				return true, nil
			},
			CanFreezeFunc: func(_ context.Context, _ *v1alpha2.VirtualMachine) (bool, error) {
				return false, nil
			},
			UnfreezeFunc: func(ctx context.Context, _, _ string) error {
				return nil
			},
			GetSecretFunc: func(_ context.Context, _, _ string) (*corev1.Secret, error) {
				return secret, nil
			},
			GetVirtualDiskSnapshotFunc: func(_ context.Context, _, _ string) (*v1alpha2.VirtualDiskSnapshot, error) {
				return vdSnapshot, nil
			},
		}

		var err error
		fakeClient, err = testutil.NewFakeClientWithObjects(vd, vm, secret, vmSnapshot, vdSnapshot)
		Expect(err).NotTo(HaveOccurred())

		recorder = &eventrecord.EventRecorderLoggerMock{
			EventFunc: func(_ client.Object, _, _, _ string) {},
		}
	})

	Context("The block devices of the virtual machine are not in the consistent state", func() {
		It("The BlockDevicesReady condition of the virtual machine isn't True", func() {
			snapshotter.GetVirtualMachineFunc = func(_ context.Context, _, _ string) (*v1alpha2.VirtualMachine, error) {
				cb := conditions.NewConditionBuilder(vmcondition.TypeBlockDevicesReady).
					Generation(vm.Generation).
					Status(metav1.ConditionFalse)
				conditions.SetCondition(cb, &vm.Status.Conditions)
				return vm, nil
			}
			h := NewLifeCycleHandler(recorder, snapshotter, storer, fakeClient)

			_, err := h.Handle(testContext(), vmSnapshot)
			Expect(err).To(BeNil())
			Expect(vmSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualMachineSnapshotPhaseFailed))
			ready, _ := conditions.GetCondition(vmscondition.VirtualMachineSnapshotReadyType, vmSnapshot.Status.Conditions)
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(vmscondition.BlockDevicesNotReady.String()))
			Expect(ready.Message).ToNot(BeEmpty())
		})

		It("The virtual disk is Pending", func() {
			snapshotter.GetVirtualDiskFunc = func(_ context.Context, _, _ string) (*v1alpha2.VirtualDisk, error) {
				vd.Status.Phase = v1alpha2.DiskPending
				return vd, nil
			}
			h := NewLifeCycleHandler(recorder, snapshotter, storer, fakeClient)

			_, err := h.Handle(testContext(), vmSnapshot)
			Expect(err).To(BeNil())
			Expect(vmSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualMachineSnapshotPhaseFailed))
			ready, _ := conditions.GetCondition(vmscondition.VirtualMachineSnapshotReadyType, vmSnapshot.Status.Conditions)
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(vmscondition.BlockDevicesNotReady.String()))
			Expect(ready.Message).ToNot(BeEmpty())
		})

		It("The virtual disk is not Ready", func() {
			snapshotter.GetVirtualDiskFunc = func(_ context.Context, _, _ string) (*v1alpha2.VirtualDisk, error) {
				cb := conditions.NewConditionBuilder(vdcondition.Ready).
					Generation(vd.Generation).
					Status(metav1.ConditionFalse)
				conditions.SetCondition(cb, &vd.Status.Conditions)
				return vd, nil
			}
			h := NewLifeCycleHandler(recorder, snapshotter, storer, fakeClient)

			_, err := h.Handle(testContext(), vmSnapshot)
			Expect(err).To(BeNil())
			Expect(vmSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualMachineSnapshotPhaseFailed))
			ready, _ := conditions.GetCondition(vmscondition.VirtualMachineSnapshotReadyType, vmSnapshot.Status.Conditions)
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(vmscondition.BlockDevicesNotReady.String()))
			Expect(ready.Message).ToNot(BeEmpty())
		})

		It("The virtual disk is the process of Resizing", func() {
			snapshotter.GetVirtualDiskFunc = func(_ context.Context, _, _ string) (*v1alpha2.VirtualDisk, error) {
				cb := conditions.NewConditionBuilder(vdcondition.ResizingType).
					Generation(vd.Generation).
					Status(metav1.ConditionTrue).
					Reason(vdcondition.InProgress)
				conditions.SetCondition(cb, &vd.Status.Conditions)
				return vd, nil
			}
			h := NewLifeCycleHandler(recorder, snapshotter, storer, fakeClient)

			_, err := h.Handle(testContext(), vmSnapshot)
			Expect(err).To(BeNil())
			Expect(vmSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualMachineSnapshotPhaseFailed))
			ready, _ := conditions.GetCondition(vmscondition.VirtualMachineSnapshotReadyType, vmSnapshot.Status.Conditions)
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(vmscondition.BlockDevicesNotReady.String()))
			Expect(ready.Message).ToNot(BeEmpty())
		})
	})

	Context("Ensure the virtual machine consistency", func() {
		It("The virtual machine has RestartAwaitingChanges", func() {
			snapshotter.GetVirtualMachineFunc = func(ctx context.Context, _, _ string) (*v1alpha2.VirtualMachine, error) {
				vm.Status.RestartAwaitingChanges = []apiextensionsv1.JSON{{}, {}}
				return vm, nil
			}

			h := NewLifeCycleHandler(recorder, snapshotter, storer, fakeClient)

			_, err := h.Handle(testContext(), vmSnapshot)
			Expect(err).To(BeNil())
			Expect(vmSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualMachineSnapshotPhaseInProgress))
			ready, _ := conditions.GetCondition(vmscondition.VirtualMachineSnapshotReadyType, vmSnapshot.Status.Conditions)
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(vmscondition.FileSystemFreezing.String()))
			Expect(ready.Message).To(Equal("The snapshotting process has started."))
		})

		It("The virtual machine is potentially inconsistent", func() {
			snapshotter.IsFrozenFunc = func(_ context.Context, _ *v1alpha2.VirtualMachine) (bool, error) {
				return false, nil
			}
			snapshotter.CanFreezeFunc = func(_ context.Context, _ *v1alpha2.VirtualMachine) (bool, error) {
				return false, nil
			}

			h := NewLifeCycleHandler(recorder, snapshotter, storer, fakeClient)

			_, err := h.Handle(testContext(), vmSnapshot)
			Expect(err).To(BeNil())
			Expect(vmSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualMachineSnapshotPhasePending))
			ready, _ := conditions.GetCondition(vmscondition.VirtualMachineSnapshotReadyType, vmSnapshot.Status.Conditions)
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(vmscondition.PotentiallyInconsistent.String()))
			Expect(ready.Message).ToNot(BeEmpty())
		})

		It("The virtual machine has frozen", func() {
			snapshotter.IsFrozenFunc = func(_ context.Context, _ *v1alpha2.VirtualMachine) (bool, error) {
				return false, nil
			}
			snapshotter.CanFreezeFunc = func(_ context.Context, _ *v1alpha2.VirtualMachine) (bool, error) {
				return true, nil
			}
			snapshotter.FreezeFunc = func(_ context.Context, _, _ string) error {
				return nil
			}

			h := NewLifeCycleHandler(recorder, snapshotter, storer, fakeClient)

			_, err := h.Handle(testContext(), vmSnapshot)
			Expect(err).To(BeNil())
			Expect(vmSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualMachineSnapshotPhaseInProgress))
			ready, _ := conditions.GetCondition(vmscondition.VirtualMachineSnapshotReadyType, vmSnapshot.Status.Conditions)
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(vmscondition.FileSystemFreezing.String()))
			Expect(ready.Message).ToNot(BeEmpty())
		})
	})

	Context("The virtual machine snapshot is Ready", func() {
		BeforeEach(func() {
			vmSnapshot.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseInProgress
		})

		It("The snapshot of virtual machine is Ready", func() {
			h := NewLifeCycleHandler(recorder, snapshotter, storer, fakeClient)

			_, err := h.Handle(testContext(), vmSnapshot)
			Expect(err).To(BeNil())
			Expect(vmSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualMachineSnapshotPhaseReady))
			ready, _ := conditions.GetCondition(vmscondition.VirtualMachineSnapshotReadyType, vmSnapshot.Status.Conditions)
			Expect(ready.Status).To(Equal(metav1.ConditionTrue))
			Expect(ready.Reason).To(Equal(vmscondition.VirtualMachineReady.String()))
			Expect(ready.Message).To(BeEmpty())

			Expect(vmSnapshot.Status.VirtualDiskSnapshotNames[0]).To(Equal(vdSnapshot.Name))
		})

		It("The snapshot of running virtual machine is consistent", func() {
			h := NewLifeCycleHandler(recorder, snapshotter, storer, fakeClient)

			_, err := h.Handle(testContext(), vmSnapshot)
			Expect(err).To(BeNil())
			Expect(vmSnapshot.Status.Consistent).To(Equal(ptr.To(true)))
		})

		It("The snapshot of stopped virtual machine is consistent", func() {
			snapshotter.GetVirtualMachineFunc = func(ctx context.Context, name, namespace string) (*v1alpha2.VirtualMachine, error) {
				vm.Status.Phase = v1alpha2.MachineStopped
				return vm, nil
			}
			h := NewLifeCycleHandler(recorder, snapshotter, storer, fakeClient)

			_, err := h.Handle(testContext(), vmSnapshot)
			Expect(err).To(BeNil())
			Expect(vmSnapshot.Status.Consistent).To(Equal(ptr.To(true)))
		})

		It("The virtual machine snapshot is potentially inconsistent", func() {
			vmSnapshot.Spec.RequiredConsistency = false
			snapshotter.GetVirtualDiskSnapshotFunc = func(_ context.Context, _, _ string) (*v1alpha2.VirtualDiskSnapshot, error) {
				vdSnapshot.Status.Consistent = nil
				return vdSnapshot, nil
			}
			h := NewLifeCycleHandler(recorder, snapshotter, storer, fakeClient)

			_, err := h.Handle(testContext(), vmSnapshot)
			Expect(err).To(BeNil())
			Expect(vmSnapshot.Status.Consistent).To(BeNil())
		})
	})
})
