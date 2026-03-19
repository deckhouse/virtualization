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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdscondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

var _ = Describe("VirtualDiskReady handler", func() {
	var snapshotter *VirtualDiskReadySnapshotterMock
	var vd *v1alpha2.VirtualDisk
	var vdSnapshot *v1alpha2.VirtualDiskSnapshot
	var fakeClient client.Client

	BeforeEach(func() {
		var err error
		fakeClient, err = testutil.NewFakeClientWithObjects()
		Expect(err).NotTo(HaveOccurred())

		vd = &v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{Name: "vd-01"},
			Status: v1alpha2.VirtualDiskStatus{
				Phase: v1alpha2.DiskReady,
				Conditions: []metav1.Condition{
					{
						Type:   vdcondition.SnapshottingType.String(),
						Status: metav1.ConditionTrue,
					},
				},
			},
		}

		vdSnapshot = &v1alpha2.VirtualDiskSnapshot{
			ObjectMeta: metav1.ObjectMeta{Name: "vdsnapshot"},
			Spec:       v1alpha2.VirtualDiskSnapshotSpec{VirtualDiskName: vd.Name},
		}

		snapshotter = &VirtualDiskReadySnapshotterMock{
			GetVirtualDiskFunc: func(_ context.Context, _, _ string) (*v1alpha2.VirtualDisk, error) {
				return vd, nil
			},
		}
	})

	Context("condition VirtualDiskReady is Unknown", func() {
		It("The virtual disk snapshot is being deleted", func() {
			vdSnapshot.DeletionTimestamp = ptr.To(metav1.Now())
			h := NewVirtualDiskReadyHandler(snapshotter, fakeClient)

			_, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			ready, _ := conditions.GetCondition(vdscondition.VirtualDiskReadyType, vdSnapshot.Status.Conditions)
			Expect(ready.Status).To(Equal(metav1.ConditionUnknown))
			Expect(ready.Reason).To(Equal(conditions.ReasonUnknown.String()))
			Expect(ready.Message).To(BeEmpty())
		})
	})

	Context("condition VirtualDiskReady is True", func() {
		It("The virtual disk is ready for snapshotting", func() {
			h := NewVirtualDiskReadyHandler(snapshotter, fakeClient)

			_, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			ready, _ := conditions.GetCondition(vdscondition.VirtualDiskReadyType, vdSnapshot.Status.Conditions)
			Expect(ready.Status).To(Equal(metav1.ConditionTrue))
			Expect(ready.Reason).To(Equal(vdscondition.VirtualDiskReady.String()))
			Expect(ready.Message).To(BeEmpty())
		})

		It("The virtual disk snapshot is already in Ready phase", func() {
			vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseReady
			h := NewVirtualDiskReadyHandler(snapshotter, fakeClient)

			_, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			ready, _ := conditions.GetCondition(vdscondition.VirtualDiskReadyType, vdSnapshot.Status.Conditions)
			Expect(ready.Status).To(Equal(metav1.ConditionTrue))
			Expect(ready.Reason).To(Equal(vdscondition.VirtualDiskReady.String()))
			Expect(ready.Message).To(BeEmpty())
		})

		It("The virtual disk has no snapshotting condition", func() {
			snapshotter.GetVirtualDiskFunc = func(_ context.Context, _, _ string) (*v1alpha2.VirtualDisk, error) {
				vd.Status.Conditions = nil
				return vd, nil
			}
			h := NewVirtualDiskReadyHandler(snapshotter, fakeClient)

			_, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			ready, _ := conditions.GetCondition(vdscondition.VirtualDiskReadyType, vdSnapshot.Status.Conditions)
			Expect(ready.Status).To(Equal(metav1.ConditionTrue))
			Expect(ready.Reason).To(Equal(vdscondition.VirtualDiskReady.String()))
			Expect(ready.Message).To(BeEmpty())
		})

		It("The virtual disk is InUse but attached VM is not migrating", func() {
			const namespace = "default"
			vmName := "vm-running"
			vd.Namespace = namespace
			vd.Status.Conditions = []metav1.Condition{
				{Type: vdcondition.SnapshottingType.String(), Status: metav1.ConditionTrue},
				{Type: vdcondition.InUseType.String(), Status: metav1.ConditionTrue},
			}
			vd.Status.AttachedToVirtualMachines = []v1alpha2.AttachedVirtualMachine{{Name: vmName, Mounted: true}}
			vdSnapshot.Namespace = namespace

			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Name: vmName, Namespace: namespace},
				Status:     v1alpha2.VirtualMachineStatus{},
			}
			fakeClient, err := testutil.NewFakeClientWithObjects(vm)
			Expect(err).NotTo(HaveOccurred())

			snapshotter.GetVirtualDiskFunc = func(_ context.Context, _, _ string) (*v1alpha2.VirtualDisk, error) {
				return vd, nil
			}
			h := NewVirtualDiskReadyHandler(snapshotter, fakeClient)

			_, err = h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			ready, _ := conditions.GetCondition(vdscondition.VirtualDiskReadyType, vdSnapshot.Status.Conditions)
			Expect(ready.Status).To(Equal(metav1.ConditionTrue))
			Expect(ready.Reason).To(Equal(vdscondition.VirtualDiskReady.String()))
			Expect(ready.Message).To(BeEmpty())
		})

		It("The virtual disk has multiple attached VMs but only one is mounted and it is not migrating", func() {
			const namespace = "default"
			vmName := "vm-mounted"
			vd.Namespace = namespace
			vd.Status.Conditions = []metav1.Condition{
				{Type: vdcondition.SnapshottingType.String(), Status: metav1.ConditionTrue},
				{Type: vdcondition.InUseType.String(), Status: metav1.ConditionTrue},
			}
			vd.Status.AttachedToVirtualMachines = []v1alpha2.AttachedVirtualMachine{
				{Name: "vm-unmounted", Mounted: false},
				{Name: vmName, Mounted: true},
			}
			vdSnapshot.Namespace = namespace

			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Name: vmName, Namespace: namespace},
				Status:     v1alpha2.VirtualMachineStatus{},
			}
			fakeClient, err := testutil.NewFakeClientWithObjects(vm)
			Expect(err).NotTo(HaveOccurred())

			snapshotter.GetVirtualDiskFunc = func(_ context.Context, _, _ string) (*v1alpha2.VirtualDisk, error) {
				return vd, nil
			}
			h := NewVirtualDiskReadyHandler(snapshotter, fakeClient)

			_, err = h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			ready, _ := conditions.GetCondition(vdscondition.VirtualDiskReadyType, vdSnapshot.Status.Conditions)
			Expect(ready.Status).To(Equal(metav1.ConditionTrue))
			Expect(ready.Reason).To(Equal(vdscondition.VirtualDiskReady.String()))
			Expect(ready.Message).To(BeEmpty())
		})
	})

	Context("condition VirtualDiskReady is False", func() {
		It("The virtual disk not found", func() {
			snapshotter.GetVirtualDiskFunc = func(_ context.Context, _, _ string) (*v1alpha2.VirtualDisk, error) {
				return nil, nil
			}
			h := NewVirtualDiskReadyHandler(snapshotter, fakeClient)

			_, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			ready, _ := conditions.GetCondition(vdscondition.VirtualDiskReadyType, vdSnapshot.Status.Conditions)
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(vdscondition.VirtualDiskNotReadyForSnapshotting.String()))
			Expect(ready.Message).ToNot(BeEmpty())
		})

		It("The virtual disk is in process of deletion", func() {
			snapshotter.GetVirtualDiskFunc = func(_ context.Context, _, _ string) (*v1alpha2.VirtualDisk, error) {
				vd.DeletionTimestamp = ptr.To(metav1.Now())
				return vd, nil
			}
			h := NewVirtualDiskReadyHandler(snapshotter, fakeClient)

			_, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			ready, _ := conditions.GetCondition(vdscondition.VirtualDiskReadyType, vdSnapshot.Status.Conditions)
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(vdscondition.VirtualDiskNotReadyForSnapshotting.String()))
			Expect(ready.Message).ToNot(BeEmpty())
		})

		It("The virtual disk is not Ready", func() {
			snapshotter.GetVirtualDiskFunc = func(_ context.Context, _, _ string) (*v1alpha2.VirtualDisk, error) {
				vd.Status.Phase = v1alpha2.DiskPending
				return vd, nil
			}
			h := NewVirtualDiskReadyHandler(snapshotter, fakeClient)

			_, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			ready, _ := conditions.GetCondition(vdscondition.VirtualDiskReadyType, vdSnapshot.Status.Conditions)
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(vdscondition.VirtualDiskNotReadyForSnapshotting.String()))
			Expect(ready.Message).ToNot(BeEmpty())
		})

		It("The virtual disk is not ready for snapshot taking yet", func() {
			snapshotter.GetVirtualDiskFunc = func(_ context.Context, _, _ string) (*v1alpha2.VirtualDisk, error) {
				vd.Status.Conditions = nil
				vd.Status.Conditions = append(vd.Status.Conditions, metav1.Condition{
					Type:    vdcondition.SnapshottingType.String(),
					Status:  metav1.ConditionFalse,
					Reason:  vdscondition.VirtualDiskNotReadyForSnapshotting.String(),
					Message: "error",
				})
				return vd, nil
			}
			h := NewVirtualDiskReadyHandler(snapshotter, fakeClient)

			_, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			ready, _ := conditions.GetCondition(vdscondition.VirtualDiskReadyType, vdSnapshot.Status.Conditions)
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(vdscondition.VirtualDiskNotReadyForSnapshotting.String()))
			Expect(ready.Message).ToNot(BeEmpty())
		})

		It("The virtual disk snapshotting condition is stale", func() {
			snapshotter.GetVirtualDiskFunc = func(_ context.Context, _, _ string) (*v1alpha2.VirtualDisk, error) {
				vd.Generation = 2
				vd.Status.Conditions = []metav1.Condition{
					{
						Type:               vdcondition.SnapshottingType.String(),
						Status:             metav1.ConditionTrue,
						ObservedGeneration: 1,
					},
				}
				return vd, nil
			}
			h := NewVirtualDiskReadyHandler(snapshotter, fakeClient)

			_, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			ready, _ := conditions.GetCondition(vdscondition.VirtualDiskReadyType, vdSnapshot.Status.Conditions)
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(vdscondition.VirtualDiskNotReadyForSnapshotting.String()))
		})

		It("The virtual disk is attached to a migrating VM", func() {
			const namespace = "default"
			vmName := "vm-migrating"
			vd.Namespace = namespace
			vd.Status.Conditions = []metav1.Condition{
				{Type: vdcondition.SnapshottingType.String(), Status: metav1.ConditionTrue},
				{Type: vdcondition.InUseType.String(), Status: metav1.ConditionTrue},
			}
			vd.Status.AttachedToVirtualMachines = []v1alpha2.AttachedVirtualMachine{{Name: vmName, Mounted: true}}
			vdSnapshot.Namespace = namespace

			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Name: vmName, Namespace: namespace},
				Status: v1alpha2.VirtualMachineStatus{
					Conditions: []metav1.Condition{
						{Type: vmcondition.TypeMigrating.String(), Status: metav1.ConditionTrue},
					},
				},
			}
			fakeClient, err := testutil.NewFakeClientWithObjects(vm)
			Expect(err).NotTo(HaveOccurred())

			snapshotter.GetVirtualDiskFunc = func(_ context.Context, _, _ string) (*v1alpha2.VirtualDisk, error) {
				return vd, nil
			}
			h := NewVirtualDiskReadyHandler(snapshotter, fakeClient)

			_, err = h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			ready, _ := conditions.GetCondition(vdscondition.VirtualDiskReadyType, vdSnapshot.Status.Conditions)
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(vdscondition.VirtualDiskNotReadyForSnapshotting.String()))
			Expect(ready.Message).To(ContainSubstring("migrating"))
		})
	})

	Context("returns an error", func() {
		It("The virtual disk is InUse but has no mounted VMs", func() {
			const namespace = "default"
			vd.Namespace = namespace
			vd.Status.Conditions = []metav1.Condition{
				{Type: vdcondition.SnapshottingType.String(), Status: metav1.ConditionTrue},
				{Type: vdcondition.InUseType.String(), Status: metav1.ConditionTrue},
			}
			vd.Status.AttachedToVirtualMachines = []v1alpha2.AttachedVirtualMachine{
				{Name: "vm-01", Mounted: false},
			}
			vdSnapshot.Namespace = namespace

			snapshotter.GetVirtualDiskFunc = func(_ context.Context, _, _ string) (*v1alpha2.VirtualDisk, error) {
				return vd, nil
			}
			h := NewVirtualDiskReadyHandler(snapshotter, fakeClient)

			_, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("please report a bug"))
		})

		It("The virtual disk is InUse with multiple mounted VMs", func() {
			const namespace = "default"
			vd.Namespace = namespace
			vd.Status.Conditions = []metav1.Condition{
				{Type: vdcondition.SnapshottingType.String(), Status: metav1.ConditionTrue},
				{Type: vdcondition.InUseType.String(), Status: metav1.ConditionTrue},
			}
			vd.Status.AttachedToVirtualMachines = []v1alpha2.AttachedVirtualMachine{
				{Name: "vm-01", Mounted: true},
				{Name: "vm-02", Mounted: true},
			}
			vdSnapshot.Namespace = namespace

			snapshotter.GetVirtualDiskFunc = func(_ context.Context, _, _ string) (*v1alpha2.VirtualDisk, error) {
				return vd, nil
			}
			h := NewVirtualDiskReadyHandler(snapshotter, fakeClient)

			_, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("please report a bug"))
		})
	})
})
