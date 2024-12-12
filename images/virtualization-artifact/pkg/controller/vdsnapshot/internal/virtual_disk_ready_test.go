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

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdscondition"
)

var _ = Describe("VirtualDiskReady handler", func() {
	var snapshotter *VirtualDiskReadySnapshotterMock
	var vd *virtv2.VirtualDisk
	var vdSnapshot *virtv2.VirtualDiskSnapshot

	BeforeEach(func() {
		vd = &virtv2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{Name: "vd-01"},
			Status: virtv2.VirtualDiskStatus{
				Phase: virtv2.DiskReady,
				Conditions: []metav1.Condition{
					{
						Type:   vdcondition.SnapshottingType.String(),
						Status: metav1.ConditionTrue,
					},
				},
			},
		}

		vdSnapshot = &virtv2.VirtualDiskSnapshot{
			ObjectMeta: metav1.ObjectMeta{Name: "vdsnapshot"},
			Spec:       virtv2.VirtualDiskSnapshotSpec{VirtualDiskName: vd.Name},
		}

		snapshotter = &VirtualDiskReadySnapshotterMock{
			GetVirtualDiskFunc: func(_ context.Context, _, _ string) (*virtv2.VirtualDisk, error) {
				return vd, nil
			},
		}
	})

	Context("condition VirtualDiskReady is True", func() {
		It("The virtual disk is ready for snapshotting", func() {
			h := NewVirtualDiskReadyHandler(snapshotter)

			_, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			ready, _ := conditions.GetCondition(vdscondition.VirtualDiskReadyType, vdSnapshot.Status.Conditions)
			Expect(ready.Status).To(Equal(metav1.ConditionTrue))
			Expect(ready.Reason).To(Equal(vdscondition.VirtualDiskReady.String()))
			Expect(ready.Message).To(BeEmpty())
		})
	})

	Context("condition VirtualDiskReady is False", func() {
		It("The virtual disk not found", func() {
			snapshotter.GetVirtualDiskFunc = func(_ context.Context, _, _ string) (*virtv2.VirtualDisk, error) {
				return nil, nil
			}
			h := NewVirtualDiskReadyHandler(snapshotter)

			_, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			ready, _ := conditions.GetCondition(vdscondition.VirtualDiskReadyType, vdSnapshot.Status.Conditions)
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(vdscondition.VirtualDiskNotReadyForSnapshotting.String()))
			Expect(ready.Message).ToNot(BeEmpty())
		})

		It("The virtual disk is in process of deletion", func() {
			snapshotter.GetVirtualDiskFunc = func(_ context.Context, _, _ string) (*virtv2.VirtualDisk, error) {
				vd.DeletionTimestamp = ptr.To(metav1.Now())
				return vd, nil
			}
			h := NewVirtualDiskReadyHandler(snapshotter)

			_, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			ready, _ := conditions.GetCondition(vdscondition.VirtualDiskReadyType, vdSnapshot.Status.Conditions)
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(vdscondition.VirtualDiskNotReadyForSnapshotting.String()))
			Expect(ready.Message).ToNot(BeEmpty())
		})

		It("The virtual disk is not Ready", func() {
			snapshotter.GetVirtualDiskFunc = func(_ context.Context, _, _ string) (*virtv2.VirtualDisk, error) {
				vd.Status.Phase = virtv2.DiskPending
				return vd, nil
			}
			h := NewVirtualDiskReadyHandler(snapshotter)

			_, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			ready, _ := conditions.GetCondition(vdscondition.VirtualDiskReadyType, vdSnapshot.Status.Conditions)
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(vdscondition.VirtualDiskNotReadyForSnapshotting.String()))
			Expect(ready.Message).ToNot(BeEmpty())
		})

		It("The virtual disk is not ready for snapshot taking yet", func() {
			snapshotter.GetVirtualDiskFunc = func(_ context.Context, _, _ string) (*virtv2.VirtualDisk, error) {
				vd.Status.Conditions = nil
				vd.Status.Conditions = append(vd.Status.Conditions, metav1.Condition{
					Type:    vdcondition.SnapshottingType.String(),
					Status:  metav1.ConditionFalse,
					Reason:  vdscondition.VirtualDiskNotReadyForSnapshotting.String(),
					Message: "error",
				})
				return vd, nil
			}
			h := NewVirtualDiskReadyHandler(snapshotter)

			_, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			ready, _ := conditions.GetCondition(vdscondition.VirtualDiskReadyType, vdSnapshot.Status.Conditions)
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(vdscondition.VirtualDiskNotReadyForSnapshotting.String()))
			Expect(ready.Message).ToNot(BeEmpty())
		})
	})
})
