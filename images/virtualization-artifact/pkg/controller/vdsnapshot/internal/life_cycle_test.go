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

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdscondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

var _ = Describe("LifeCycle handler", func() {
	var snapshotter *LifeCycleSnapshotterMock
	var pvc *corev1.PersistentVolumeClaim
	var vd *virtv2.VirtualDisk
	var vs *vsv1.VolumeSnapshot
	var vdSnapshot *virtv2.VirtualDiskSnapshot

	BeforeEach(func() {
		pvc = &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "pvc-01"},
			Status: corev1.PersistentVolumeClaimStatus{
				Phase: corev1.ClaimBound,
			},
		}

		vd = &virtv2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{Name: "vd-01"},
			Status: virtv2.VirtualDiskStatus{
				Target: virtv2.DiskTarget{
					PersistentVolumeClaim: pvc.Name,
				},
			},
		}

		vs = &vsv1.VolumeSnapshot{
			ObjectMeta: metav1.ObjectMeta{Name: "vs-01"},
		}

		vdSnapshot = &virtv2.VirtualDiskSnapshot{
			ObjectMeta: metav1.ObjectMeta{Name: "vdsnapshot"},
			Spec:       virtv2.VirtualDiskSnapshotSpec{VirtualDiskName: vd.Name},
			Status: virtv2.VirtualDiskSnapshotStatus{
				Conditions: []metav1.Condition{
					{
						Type:   vdscondition.VirtualDiskReadyType.String(),
						Status: metav1.ConditionTrue,
					},
				},
			},
		}

		snapshotter = &LifeCycleSnapshotterMock{
			CreateVolumeSnapshotFunc: func(_ context.Context, _ *vsv1.VolumeSnapshot) (*vsv1.VolumeSnapshot, error) {
				return vs, nil
			},
			GetPersistentVolumeClaimFunc: func(_ context.Context, _, _ string) (*corev1.PersistentVolumeClaim, error) {
				return pvc, nil
			},
			GetVirtualDiskFunc: func(_ context.Context, _, _ string) (*virtv2.VirtualDisk, error) {
				return vd, nil
			},
			GetVirtualMachineFunc: func(_ context.Context, _, _ string) (*virtv2.VirtualMachine, error) {
				return nil, nil
			},
			GetVolumeSnapshotFunc: func(_ context.Context, _, _ string) (*vsv1.VolumeSnapshot, error) {
				return nil, nil
			},
		}
	})

	Context("The virtual disk snapshot without virtual machine", func() {
		It("The volume snapshot has created", func() {
			h := NewLifeCycleHandler(snapshotter)

			_, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			Expect(vdSnapshot.Status.Phase).To(Equal(virtv2.VirtualDiskSnapshotPhaseInProgress))
			ready, _ := conditions.GetCondition(vdscondition.VirtualDiskSnapshotReadyType, vdSnapshot.Status.Conditions)
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(vdscondition.Snapshotting.String()))
			Expect(ready.Message).ToNot(BeEmpty())
		})

		It("The volume snapshot has failed", func() {
			snapshotter.GetVolumeSnapshotFunc = func(_ context.Context, _, _ string) (*vsv1.VolumeSnapshot, error) {
				vs.Status = &vsv1.VolumeSnapshotStatus{
					Error: &vsv1.VolumeSnapshotError{
						Message: ptr.To("error"),
					},
				}
				return vs, nil
			}

			h := NewLifeCycleHandler(snapshotter)

			_, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			Expect(vdSnapshot.Status.Phase).To(Equal(virtv2.VirtualDiskSnapshotPhaseFailed))
			ready, _ := conditions.GetCondition(vdscondition.VirtualDiskSnapshotReadyType, vdSnapshot.Status.Conditions)
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(vdscondition.VirtualDiskSnapshotFailed.String()))
			Expect(ready.Message).ToNot(BeEmpty())
		})

		It("The volume snapshot is not ready yet", func() {
			snapshotter.GetVolumeSnapshotFunc = func(_ context.Context, _, _ string) (*vsv1.VolumeSnapshot, error) {
				return vs, nil
			}

			h := NewLifeCycleHandler(snapshotter)

			_, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			Expect(vdSnapshot.Status.Phase).To(Equal(virtv2.VirtualDiskSnapshotPhaseInProgress))
			ready, _ := conditions.GetCondition(vdscondition.VirtualDiskSnapshotReadyType, vdSnapshot.Status.Conditions)
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(vdscondition.Snapshotting.String()))
			Expect(ready.Message).ToNot(BeEmpty())
		})

		It("The volume snapshot is ready", func() {
			snapshotter.GetVolumeSnapshotFunc = func(_ context.Context, _, _ string) (*vsv1.VolumeSnapshot, error) {
				vs.Status = &vsv1.VolumeSnapshotStatus{
					ReadyToUse: ptr.To(true),
				}
				return vs, nil
			}

			h := NewLifeCycleHandler(snapshotter)

			_, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			Expect(vdSnapshot.Status.Phase).To(Equal(virtv2.VirtualDiskSnapshotPhaseReady))
			ready, _ := conditions.GetCondition(vdscondition.VirtualDiskSnapshotReadyType, vdSnapshot.Status.Conditions)
			Expect(ready.Status).To(Equal(metav1.ConditionTrue))
			Expect(ready.Reason).To(Equal(vdscondition.VirtualDiskSnapshotReady.String()))
			Expect(ready.Message).To(BeEmpty())
		})
	})

	Context("The virtual disk snapshot with virtual machine", func() {
		var vm *virtv2.VirtualMachine

		BeforeEach(func() {
			vdSnapshot.Status.Phase = virtv2.VirtualDiskSnapshotPhaseInProgress

			vm = &virtv2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "vm"},
				Status: virtv2.VirtualMachineStatus{
					Phase: virtv2.MachineRunning,
				},
			}
			vd.Status.AttachedToVirtualMachines = []virtv2.AttachedVirtualMachine{{Name: vm.Name}}

			snapshotter.GetVirtualMachineFunc = func(_ context.Context, _, _ string) (*virtv2.VirtualMachine, error) {
				return vm, nil
			}
			snapshotter.IsFrozenFunc = func(_ *virtv2.VirtualMachine) bool {
				return false
			}
			snapshotter.CanFreezeFunc = func(_ *virtv2.VirtualMachine) bool {
				return true
			}
			snapshotter.FreezeFunc = func(_ context.Context, _, _ string) error {
				return nil
			}
			snapshotter.CanUnfreezeWithVirtualDiskSnapshotFunc = func(_ context.Context, _ string, _ *virtv2.VirtualMachine) (bool, error) {
				return true, nil
			}
			snapshotter.UnfreezeFunc = func(_ context.Context, _, _ string) error {
				return nil
			}
		})

		It("Freeze virtual machine", func() {
			h := NewLifeCycleHandler(snapshotter)

			_, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			Expect(vdSnapshot.Status.Phase).To(Equal(virtv2.VirtualDiskSnapshotPhaseInProgress))
			ready, _ := conditions.GetCondition(vdscondition.VirtualDiskSnapshotReadyType, vdSnapshot.Status.Conditions)
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(vdscondition.FileSystemFreezing.String()))
			Expect(ready.Message).ToNot(BeEmpty())
		})

		It("No need to freeze virtual machine", func() {
			snapshotter.GetVirtualMachineFunc = func(_ context.Context, _, _ string) (*virtv2.VirtualMachine, error) {
				vm.Status.Phase = virtv2.MachineStopped
				return vm, nil
			}
			h := NewLifeCycleHandler(snapshotter)

			_, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			Expect(vdSnapshot.Status.Phase).To(Equal(virtv2.VirtualDiskSnapshotPhaseInProgress))
			ready, _ := conditions.GetCondition(vdscondition.VirtualDiskSnapshotReadyType, vdSnapshot.Status.Conditions)
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(vdscondition.Snapshotting.String()))
			Expect(ready.Message).ToNot(BeEmpty())
		})

		It("Cannot freeze virtual machine: deny potentially inconsistent", func() {
			vdSnapshot.Spec.RequiredConsistency = true
			snapshotter.CanFreezeFunc = func(_ *virtv2.VirtualMachine) bool {
				return false
			}
			h := NewLifeCycleHandler(snapshotter)

			_, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			Expect(vdSnapshot.Status.Phase).To(Equal(virtv2.VirtualDiskSnapshotPhasePending))
			ready, _ := conditions.GetCondition(vdscondition.VirtualDiskSnapshotReadyType, vdSnapshot.Status.Conditions)
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(vdscondition.PotentiallyInconsistent.String()))
			Expect(ready.Message).ToNot(BeEmpty())
		})

		It("Cannot freeze virtual machine: allow potentially inconsistent", func() {
			vdSnapshot.Spec.RequiredConsistency = false
			snapshotter.CanFreezeFunc = func(_ *virtv2.VirtualMachine) bool {
				return false
			}
			h := NewLifeCycleHandler(snapshotter)

			_, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			Expect(vdSnapshot.Status.Phase).To(Equal(virtv2.VirtualDiskSnapshotPhaseInProgress))
			ready, _ := conditions.GetCondition(vdscondition.VirtualDiskSnapshotReadyType, vdSnapshot.Status.Conditions)
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(vdscondition.Snapshotting.String()))
			Expect(ready.Message).ToNot(BeEmpty())
		})

		It("Unfreeze virtual machine", func() {
			snapshotter.IsFrozenFunc = func(_ *virtv2.VirtualMachine) bool {
				return true
			}
			snapshotter.GetVolumeSnapshotFunc = func(_ context.Context, _, _ string) (*vsv1.VolumeSnapshot, error) {
				vs.Status = &vsv1.VolumeSnapshotStatus{
					ReadyToUse: ptr.To(true),
				}
				return vs, nil
			}
			h := NewLifeCycleHandler(snapshotter)

			_, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			Expect(vdSnapshot.Status.Phase).To(Equal(virtv2.VirtualDiskSnapshotPhaseReady))
			ready, _ := conditions.GetCondition(vdscondition.VirtualDiskSnapshotReadyType, vdSnapshot.Status.Conditions)
			Expect(ready.Status).To(Equal(metav1.ConditionTrue))
			Expect(ready.Reason).To(Equal(vdscondition.VirtualDiskSnapshotReady.String()))
			Expect(ready.Message).To(BeEmpty())
		})

		DescribeTable("Check unfreeze if failed", func(vm *virtv2.VirtualMachine, expectUnfreezing bool) {
			unFreezeCalled := false

			snapshotter.IsFrozenFunc = func(_ *virtv2.VirtualMachine) bool {
				return true
			}
			snapshotter.GetVolumeSnapshotFunc = func(_ context.Context, _, _ string) (*vsv1.VolumeSnapshot, error) {
				vs.Status = &vsv1.VolumeSnapshotStatus{
					ReadyToUse: ptr.To(true),
				}
				return vs, nil
			}
			snapshotter.UnfreezeFunc = func(_ context.Context, _ string, _ string) error {
				unFreezeCalled = true
				return nil
			}
			snapshotter.GetVirtualMachineFunc = func(_ context.Context, _, _ string) (*virtv2.VirtualMachine, error) {
				return vm, nil
			}

			h := NewLifeCycleHandler(snapshotter)

			vdSnapshot.Status.Phase = virtv2.VirtualDiskSnapshotPhaseFailed
			_, err := h.Handle(testContext(), vdSnapshot)

			Expect(err).To(BeNil())
			Expect(vdSnapshot.Status.Phase).To(Equal(virtv2.VirtualDiskSnapshotPhaseFailed))
			Expect(unFreezeCalled).To(Equal(expectUnfreezing))
		},
			Entry("Has VM with frozen filesystem",
				&virtv2.VirtualMachine{
					Status: virtv2.VirtualMachineStatus{
						Conditions: []metav1.Condition{
							{
								Type:   vmcondition.TypeFilesystemFrozen.String(),
								Status: metav1.ConditionTrue,
							},
						},
					},
				},
				true,
			),
			Entry("Has VM with unfrozen filesystem", &virtv2.VirtualMachine{}, false),
			Entry("Has no VM", nil, false),
		)
	})
})
