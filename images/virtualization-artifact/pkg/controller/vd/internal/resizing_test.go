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
	"log/slog"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

var _ = Describe("Resizing handler Run", func() {
	var vd *virtv2.VirtualDisk
	var pvc *corev1.PersistentVolumeClaim
	var diskService *DiskServiceMock

	size := resource.MustParse("10G")

	BeforeEach(func() {
		vd = &virtv2.VirtualDisk{
			Spec: virtv2.VirtualDiskSpec{
				PersistentVolumeClaim: virtv2.VirtualDiskPersistentVolumeClaim{
					Size: &size,
				},
			},
			Status: virtv2.VirtualDiskStatus{
				Conditions: []metav1.Condition{
					{
						Type:   vdcondition.ReadyType.String(),
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
			GetPersistentVolumeClaimFunc: func(ctx context.Context, sup *supplements.Generator) (*corev1.PersistentVolumeClaim, error) {
				return pvc, nil
			},
			ResizeFunc: func(ctx context.Context, pvc *corev1.PersistentVolumeClaim, newSize resource.Quantity) error {
				return nil
			},
		}
	})

	It("Resizing is in progress", func() {
		vd.Spec.PersistentVolumeClaim.Size = nil
		diskService.GetPersistentVolumeClaimFunc = func(ctx context.Context, sup *supplements.Generator) (*corev1.PersistentVolumeClaim, error) {
			pvc.Status.Conditions = []corev1.PersistentVolumeClaimCondition{
				{
					Type:   corev1.PersistentVolumeClaimResizing,
					Status: corev1.ConditionTrue,
				},
			}
			return pvc, nil
		}

		h := NewResizingHandler(diskService)

		_, err := h.Handle(testContext(), vd)
		Expect(err).To(BeNil())
		resized, _ := conditions.GetCondition(vdcondition.ResizedType, vd.Status.Conditions)
		Expect(resized.Status).To(Equal(metav1.ConditionFalse))
		Expect(resized.Reason).To(Equal(vdcondition.InProgress))
	})

	It("Resize is not requested (vd.spec.size == nil)", func() {
		vd.Spec.PersistentVolumeClaim.Size = nil

		h := NewResizingHandler(diskService)

		_, err := h.Handle(testContext(), vd)
		Expect(err).To(BeNil())
		resized, _ := conditions.GetCondition(vdcondition.ResizedType, vd.Status.Conditions)
		Expect(resized.Status).To(Equal(metav1.ConditionFalse))
		Expect(resized.Reason).To(Equal(vdcondition.ResizingNotRequested))
	})

	It("Resize is not requested (vd.spec.size < pvc.spec.size)", func() {
		vd.Spec.PersistentVolumeClaim.Size.Sub(resource.MustParse("1G"))

		h := NewResizingHandler(diskService)

		_, err := h.Handle(testContext(), vd)
		Expect(err).To(BeNil())
		resized, _ := conditions.GetCondition(vdcondition.ResizedType, vd.Status.Conditions)
		Expect(resized.Status).To(Equal(metav1.ConditionFalse))
		Expect(resized.Reason).To(Equal(vdcondition.ResizingNotRequested))
	})

	It("Resize is not requested (vd.spec.size == pvc.spec.size)", func() {
		h := NewResizingHandler(diskService)

		_, err := h.Handle(testContext(), vd)
		Expect(err).To(BeNil())
		resized, _ := conditions.GetCondition(vdcondition.ResizedType, vd.Status.Conditions)
		Expect(resized.Status).To(Equal(metav1.ConditionFalse))
		Expect(resized.Reason).To(Equal(vdcondition.ResizingNotRequested))
	})

	It("Resize has started (vd.spec.size > pvc.spec.size)", func() {
		vd.Spec.PersistentVolumeClaim.Size.Add(size)

		h := NewResizingHandler(diskService)

		_, err := h.Handle(testContext(), vd)
		Expect(err).To(BeNil())
		resized, _ := conditions.GetCondition(vdcondition.ResizedType, vd.Status.Conditions)
		Expect(resized.Status).To(Equal(metav1.ConditionFalse))
		Expect(resized.Reason).To(Equal(vdcondition.InProgress))
	})

	It("Resize has completed", func() {
		vd.Status.Conditions = append(vd.Status.Conditions, metav1.Condition{
			Type:   vdcondition.ResizedType.String(),
			Status: metav1.ConditionFalse,
			Reason: vdcondition.InProgress.String(),
		})

		h := NewResizingHandler(diskService)

		_, err := h.Handle(testContext(), vd)
		Expect(err).To(BeNil())
		resized, _ := conditions.GetCondition(vdcondition.ResizedType, vd.Status.Conditions)
		Expect(resized.Status).To(Equal(metav1.ConditionTrue))
		Expect(resized.Reason).To(Equal(vdcondition.Resized))
	})
})

func testContext() context.Context {
	return logger.ToContext(context.Background(), slog.Default())
}
