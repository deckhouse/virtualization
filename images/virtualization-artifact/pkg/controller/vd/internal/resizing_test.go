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
	"errors"
	"log/slog"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

var _ = Describe("Resizing handler Run", func() {
	var vd *v1alpha2.VirtualDisk
	var pvc *corev1.PersistentVolumeClaim
	var diskService *DiskServiceMock

	size := resource.MustParse("10G")

	BeforeEach(func() {
		vd = &v1alpha2.VirtualDisk{
			Spec: v1alpha2.VirtualDiskSpec{
				PersistentVolumeClaim: v1alpha2.VirtualDiskPersistentVolumeClaim{
					Size: &size,
				},
			},
			Status: v1alpha2.VirtualDiskStatus{
				Conditions: []metav1.Condition{
					{
						Type:   vdcondition.ReadyType.String(),
						Status: metav1.ConditionTrue,
					},
					{
						Type:   vdcondition.StorageClassReadyType.String(),
						Status: metav1.ConditionTrue,
					},
				},
				Capacity: size.String(),
			},
		}

		pvc = &corev1.PersistentVolumeClaim{
			Spec: corev1.PersistentVolumeClaimSpec{
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: size,
					},
				},
			},
			Status: corev1.PersistentVolumeClaimStatus{
				Phase: corev1.ClaimBound,
				Capacity: corev1.ResourceList{
					corev1.ResourceStorage: size,
				},
			},
		}

		diskService = &DiskServiceMock{
			GetPersistentVolumeClaimFunc: func(ctx context.Context, sup supplements.Generator) (*corev1.PersistentVolumeClaim, error) {
				return pvc, nil
			},
			ResizeFunc: func(ctx context.Context, pvc *corev1.PersistentVolumeClaim, newSize resource.Quantity) error {
				return nil
			},
		}
	})

	recorder := &eventrecord.EventRecorderLoggerMock{
		EventFunc: func(_ client.Object, _, _, _ string) {},
	}

	It("Resizing is in progress", func() {
		vd.Spec.PersistentVolumeClaim.Size = nil
		diskService.GetPersistentVolumeClaimFunc = func(ctx context.Context, sup supplements.Generator) (*corev1.PersistentVolumeClaim, error) {
			pvc.Status.Conditions = []corev1.PersistentVolumeClaimCondition{
				{
					Type:   corev1.PersistentVolumeClaimResizing,
					Status: corev1.ConditionTrue,
				},
			}
			return pvc, nil
		}

		h := NewResizingHandler(recorder, diskService)

		_, err := h.Handle(testContext(), vd)
		Expect(err).To(BeNil())
		resized, _ := conditions.GetCondition(vdcondition.ResizingType, vd.Status.Conditions)
		Expect(resized.Status).To(Equal(metav1.ConditionTrue))
		Expect(resized.Reason).To(Equal(vdcondition.InProgress.String()))
	})

	It("Resize is not requested (vd.spec.size == nil)", func() {
		vd.Spec.PersistentVolumeClaim.Size = nil

		h := NewResizingHandler(recorder, diskService)

		_, err := h.Handle(testContext(), vd)
		Expect(err).To(BeNil())
		_, ok := conditions.GetCondition(vdcondition.ResizingType, vd.Status.Conditions)
		Expect(ok).Should(BeFalse())
	})

	It("Resize is not requested (vd.spec.size < pvc.spec.size)", func() {
		vd.Spec.PersistentVolumeClaim.Size.Sub(resource.MustParse("1G"))

		h := NewResizingHandler(recorder, diskService)

		_, err := h.Handle(testContext(), vd)
		Expect(err).To(BeNil())
		_, ok := conditions.GetCondition(vdcondition.ResizingType, vd.Status.Conditions)
		Expect(ok).Should(BeFalse())
	})

	It("Resize is not requested (vd.spec.size == pvc.spec.size)", func() {
		h := NewResizingHandler(recorder, diskService)

		_, err := h.Handle(testContext(), vd)
		Expect(err).To(BeNil())
		_, ok := conditions.GetCondition(vdcondition.ResizingType, vd.Status.Conditions)
		Expect(ok).Should(BeFalse())
	})

	It("Resize has started (vd.spec.size > pvc.spec.size)", func() {
		vd.Spec.PersistentVolumeClaim.Size.Add(size)

		h := NewResizingHandler(recorder, diskService)

		_, err := h.Handle(testContext(), vd)
		Expect(err).To(BeNil())
		resized, _ := conditions.GetCondition(vdcondition.ResizingType, vd.Status.Conditions)
		Expect(resized.Status).To(Equal(metav1.ConditionTrue))
		Expect(resized.Reason).To(Equal(vdcondition.InProgress.String()))
	})

	It("Resize has completed", func() {
		vd.Status.Conditions = append(vd.Status.Conditions, metav1.Condition{
			Type:   vdcondition.ResizingType.String(),
			Status: metav1.ConditionFalse,
			Reason: vdcondition.InProgress.String(),
		})

		h := NewResizingHandler(recorder, diskService)

		_, err := h.Handle(testContext(), vd)
		Expect(err).To(BeNil())
		_, ok := conditions.GetCondition(vdcondition.ResizingType, vd.Status.Conditions)
		Expect(ok).Should(BeFalse())
	})

	DescribeTable("Resizing handler Handle", func(args handleTestArgs) {
		vd := &v1alpha2.VirtualDisk{
			Spec: v1alpha2.VirtualDiskSpec{},
			Status: v1alpha2.VirtualDiskStatus{
				Conditions: []metav1.Condition{
					{
						Type:   vdcondition.ResizingType.String(),
						Status: metav1.ConditionUnknown,
						Reason: conditions.ReasonUnknown.String(),
					},
					{
						Type:   vdcondition.ReadyType.String(),
						Status: args.expectedReadyConditionStatus,
						Reason: conditions.ReasonUnknown.String(),
					},
					{
						Type:   vdcondition.StorageClassReadyType.String(),
						Status: metav1.ConditionTrue,
						Reason: vdcondition.StorageClassReady.String(),
					},
				},
				Phase: args.expectedVdPhase,
			},
		}

		if args.isDiskDeleting {
			vd.DeletionTimestamp = &metav1.Time{}
		}

		diskService := &DiskServiceMock{
			GetPersistentVolumeClaimFunc: func(ctx context.Context, sup supplements.Generator) (*corev1.PersistentVolumeClaim, error) {
				if args.isPVCGetError {
					return nil, errors.New("test error")
				}
				return args.pvc, nil
			},
			ResizeFunc: func(ctx context.Context, pvc *corev1.PersistentVolumeClaim, newSize resource.Quantity) error {
				return nil
			},
		}

		recorder := &eventrecord.EventRecorderLoggerMock{
			EventFunc: func(_ client.Object, _, _, _ string) {},
		}

		handler := NewResizingHandler(recorder, diskService)
		result, err := handler.Handle(testContext(), vd)
		Expect(result).To(Equal(reconcile.Result{}))
		if args.isErrorNil {
			Expect(err).To(BeNil())
		} else {
			Expect(err).To(HaveOccurred())
		}
		Expect(vd.Status.Phase).To(Equal(args.expectedVdPhase))
	},
		Entry("Virtual Disk deleting", handleTestArgs{
			isDiskDeleting:               true,
			isPVCGetError:                false,
			pvc:                          nil,
			isErrorNil:                   true,
			expectedReadyConditionStatus: metav1.ConditionUnknown,
			expectedVdPhase:              v1alpha2.DiskTerminating,
		}),
		Entry("Virtual Disk is not ready", handleTestArgs{
			isDiskDeleting:               false,
			isPVCGetError:                false,
			pvc:                          nil,
			isErrorNil:                   true,
			expectedReadyConditionStatus: metav1.ConditionFalse,
			expectedVdPhase:              v1alpha2.DiskPending,
		}),
		Entry("PVC get error", handleTestArgs{
			isDiskDeleting:               false,
			isPVCGetError:                true,
			pvc:                          nil,
			isErrorNil:                   false,
			expectedReadyConditionStatus: metav1.ConditionTrue,
			expectedVdPhase:              v1alpha2.DiskPending,
		}),
		Entry("PVC is nil", handleTestArgs{
			isDiskDeleting:               false,
			isPVCGetError:                false,
			pvc:                          nil,
			isErrorNil:                   true,
			expectedReadyConditionStatus: metav1.ConditionTrue,
			expectedVdPhase:              v1alpha2.DiskPending,
		}),
		Entry("PVC is not bound", handleTestArgs{
			isDiskDeleting: false,
			isPVCGetError:  false,
			pvc: &corev1.PersistentVolumeClaim{
				Status: corev1.PersistentVolumeClaimStatus{
					Phase: corev1.ClaimPending,
				},
			},
			isErrorNil:                   true,
			expectedReadyConditionStatus: metav1.ConditionTrue,
			expectedVdPhase:              v1alpha2.DiskPending,
		}),
		Entry("Everything is fine", handleTestArgs{
			isDiskDeleting: false,
			isPVCGetError:  false,
			pvc: &corev1.PersistentVolumeClaim{
				Status: corev1.PersistentVolumeClaimStatus{
					Phase: corev1.ClaimBound,
				},
			},
			isErrorNil:                   true,
			expectedReadyConditionStatus: metav1.ConditionTrue,
			expectedVdPhase:              v1alpha2.DiskPending,
		}),
	)

	DescribeTable("Resizing handler ResizeNeeded", func(args resizeNeededArgs) {
		vd := &v1alpha2.VirtualDisk{
			Spec: v1alpha2.VirtualDiskSpec{
				PersistentVolumeClaim: v1alpha2.VirtualDiskPersistentVolumeClaim{
					Size: ptr.To(resource.Quantity{}),
				},
			},
			Status: v1alpha2.VirtualDiskStatus{
				Conditions: []metav1.Condition{
					{
						Type:   vdcondition.ResizingType.String(),
						Status: metav1.ConditionUnknown,
						Reason: conditions.ReasonUnknown.String(),
					},
					{
						Type:   vdcondition.SnapshottingType.String(),
						Status: args.snapshottingStatus,
						Reason: conditions.ReasonUnknown.String(),
					},
					{
						Type:   vdcondition.StorageClassReadyType.String(),
						Status: args.storageClassReadyStatus,
						Reason: vdcondition.StorageClassReady.String(),
					},
				},
				Phase: v1alpha2.DiskPending,
			},
		}

		resizeCalled := false
		diskService := &DiskServiceMock{
			ResizeFunc: func(ctx context.Context, pvc *corev1.PersistentVolumeClaim, newSize resource.Quantity) error {
				resizeCalled = true
				if args.isResizeReturnErr {
					return errors.New("test error")
				}
				return nil
			},
		}

		recorder := &eventrecord.EventRecorderLoggerMock{
			EventFunc: func(_ client.Object, _, _, _ string) {},
		}

		log := logger.FromContext(testContext()).With(logger.SlogHandler("resizing"))

		handler := NewResizingHandler(recorder, diskService)
		cb := conditions.NewConditionBuilder(vdcondition.ResizingType)

		result, err := handler.ResizeNeeded(testContext(), vd, pvc, cb, log)

		Expect(result).To(Equal(reconcile.Result{}))
		if args.expectedHaveError {
			Expect(err).To(HaveOccurred())
		} else {
			Expect(err).To(BeNil())
		}
		Expect(vd.Status.Phase).To(Equal(args.expectedPhase))
		Expect(resizeCalled).To(Equal(args.expectedResizeCalled))
		Expect(cb.Condition().Status).To(Equal(args.expectedStatus))
		Expect(cb.Condition().Reason).To(Equal(args.expectedReason))
	},
		Entry("Snapshotting", resizeNeededArgs{
			snapshottingStatus:      metav1.ConditionTrue,
			storageClassReadyStatus: metav1.ConditionUnknown,
			isResizeReturnErr:       false,
			expectedResizeCalled:    false,
			expectedHaveError:       false,
			expectedPhase:           v1alpha2.DiskPending,
			expectedStatus:          metav1.ConditionFalse,
			expectedReason:          vdcondition.ResizingNotAvailable.String(),
		}),
		Entry("StorageClass not ready", resizeNeededArgs{
			snapshottingStatus:      metav1.ConditionFalse,
			storageClassReadyStatus: metav1.ConditionFalse,
			isResizeReturnErr:       false,
			expectedResizeCalled:    false,
			expectedHaveError:       false,
			expectedPhase:           v1alpha2.DiskPending,
			expectedStatus:          metav1.ConditionFalse,
			expectedReason:          vdcondition.ResizingNotAvailable.String(),
		}),
		Entry("StorageClassReady is Unknown", resizeNeededArgs{
			snapshottingStatus:      metav1.ConditionFalse,
			storageClassReadyStatus: metav1.ConditionUnknown,
			isResizeReturnErr:       false,
			expectedResizeCalled:    false,
			expectedHaveError:       false,
			expectedPhase:           v1alpha2.DiskPending,
			expectedStatus:          metav1.ConditionFalse,
			expectedReason:          vdcondition.ResizingNotAvailable.String(),
		}),
		Entry("Resize return err", resizeNeededArgs{
			snapshottingStatus:      metav1.ConditionFalse,
			storageClassReadyStatus: metav1.ConditionTrue,
			isResizeReturnErr:       true,
			expectedResizeCalled:    true,
			expectedHaveError:       true,
			expectedPhase:           v1alpha2.DiskPending,
			expectedStatus:          metav1.ConditionUnknown,
			expectedReason:          conditions.ReasonUnknown.String(),
		}),
		Entry("Positive case", resizeNeededArgs{
			snapshottingStatus:      metav1.ConditionFalse,
			storageClassReadyStatus: metav1.ConditionTrue,
			isResizeReturnErr:       false,
			expectedResizeCalled:    true,
			expectedHaveError:       false,
			expectedPhase:           v1alpha2.DiskResizing,
			expectedStatus:          metav1.ConditionTrue,
			expectedReason:          vdcondition.InProgress.String(),
		}),
	)
})

func testContext() context.Context {
	return logger.ToContext(context.Background(), slog.Default())
}

type handleTestArgs struct {
	isDiskDeleting               bool
	isPVCGetError                bool
	isErrorNil                   bool
	pvc                          *corev1.PersistentVolumeClaim
	expectedReadyConditionStatus metav1.ConditionStatus
	expectedVdPhase              v1alpha2.DiskPhase
}

type resizeNeededArgs struct {
	snapshottingStatus      metav1.ConditionStatus
	storageClassReadyStatus metav1.ConditionStatus
	isResizeReturnErr       bool
	expectedResizeCalled    bool
	expectedHaveError       bool
	expectedPhase           v1alpha2.DiskPhase
	expectedStatus          metav1.ConditionStatus
	expectedReason          string
}
