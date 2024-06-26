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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

var _ = Describe("Resizing handler Run", func() {
	var vd *virtv2.VirtualDisk
	var pvc *corev1.PersistentVolumeClaim
	var diskService *DiskServiceMock

	BeforeEach(func() {
		vd = &virtv2.VirtualDisk{
			Spec: virtv2.VirtualDiskSpec{
				PersistentVolumeClaim: virtv2.VirtualDiskPersistentVolumeClaim{
					Size: new(resource.Quantity),
				},
			},
			Status: virtv2.VirtualDiskStatus{
				Conditions: []metav1.Condition{
					{
						Type:   vdcondition.ReadyType,
						Status: metav1.ConditionTrue,
					},
				},
				Capacity: "",
			},
		}

		pvc = &corev1.PersistentVolumeClaim{
			Spec: corev1.PersistentVolumeClaimSpec{
				Resources: corev1.VolumeResourceRequirements{
					Requests: make(corev1.ResourceList),
				},
			},
			Status: corev1.PersistentVolumeClaimStatus{
				Capacity: make(corev1.ResourceList),
			},
		}

		diskService = &DiskServiceMock{
			GetPersistentVolumeClaimFunc: func(ctx context.Context, sup *supplements.Generator) (*corev1.PersistentVolumeClaim, error) {
				return nil, nil
			},
			ResizeFunc: func(ctx context.Context, pvc *corev1.PersistentVolumeClaim, newSize resource.Quantity) error {
				return nil
			},
		}
	})

	It("Resize is not requested (vd.spec.size == nil)", func() {
		vd.Spec.PersistentVolumeClaim.Size = nil

		h := NewResizingHandler(diskService)

		_, err := h.Handle(context.Background(), vd)
		Expect(err).To(BeNil())
		Expect(vd.Status.Conditions).To(ContainElement(metav1.Condition{
			Type:   vdcondition.ResizedType,
			Status: metav1.ConditionFalse,
			Reason: vdcondition.NotRequested,
		}))
	})

	It("Resize is not requested (vd.spec.size < pvc.spec.size)", func() {
		*vd.Spec.PersistentVolumeClaim.Size = resource.MustParse("1G")
		diskService.GetPersistentVolumeClaimFunc = func(ctx context.Context, sup *supplements.Generator) (*corev1.PersistentVolumeClaim, error) {
			pvc.Spec.Resources.Requests[corev1.ResourceStorage] = resource.MustParse("2G")
			return pvc, nil
		}

		h := NewResizingHandler(diskService)

		_, err := h.Handle(context.Background(), vd)
		Expect(err).To(BeNil())
		Expect(vd.Status.Conditions).To(ContainElement(metav1.Condition{
			Type:   vdcondition.ResizedType,
			Status: metav1.ConditionFalse,
			Reason: vdcondition.NotRequested,
		}))
	})

	It("Resize has started (vd.spec.size > pvc.spec.size)", func() {
		*vd.Spec.PersistentVolumeClaim.Size = resource.MustParse("2G")
		diskService.GetPersistentVolumeClaimFunc = func(ctx context.Context, sup *supplements.Generator) (*corev1.PersistentVolumeClaim, error) {
			pvc.Spec.Resources.Requests[corev1.ResourceStorage] = resource.MustParse("1G")
			return pvc, nil
		}

		h := NewResizingHandler(diskService)

		_, err := h.Handle(context.Background(), vd)
		Expect(err).To(BeNil())

		resized, _ := service.GetCondition(vdcondition.ResizedType, vd.Status.Conditions)
		Expect(resized.Status).To(Equal(metav1.ConditionFalse))
		Expect(resized.Reason).To(Equal(vdcondition.InProgress))
	})

	It("Resize is in progress (vd.spec.size == pvc.spec.size, pvc.spec.size > pvc.status.size)", func() {
		*vd.Spec.PersistentVolumeClaim.Size = resource.MustParse("2G")
		q := resource.MustParse("1G")
		vd.Status.Capacity = q.String()
		diskService.GetPersistentVolumeClaimFunc = func(ctx context.Context, sup *supplements.Generator) (*corev1.PersistentVolumeClaim, error) {
			pvc.Spec.Resources.Requests[corev1.ResourceStorage] = resource.MustParse("2G")
			pvc.Status.Capacity[corev1.ResourceStorage] = resource.MustParse("1G")
			return pvc, nil
		}

		h := NewResizingHandler(diskService)

		_, err := h.Handle(context.Background(), vd)
		Expect(err).To(BeNil())

		resized, _ := service.GetCondition(vdcondition.ResizedType, vd.Status.Conditions)
		Expect(resized.Status).To(Equal(metav1.ConditionFalse))
		Expect(resized.Reason).To(Equal(vdcondition.InProgress))
	})

	It("Resized (vd.spec.size == pvc.spec.size, pvc.spec.size == pvc.status.size)", func() {
		*vd.Spec.PersistentVolumeClaim.Size = resource.MustParse("2G")
		q := resource.MustParse("1G")
		vd.Status.Capacity = q.String()
		diskService.GetPersistentVolumeClaimFunc = func(ctx context.Context, sup *supplements.Generator) (*corev1.PersistentVolumeClaim, error) {
			pvc.Spec.Resources.Requests[corev1.ResourceStorage] = resource.MustParse("2G")
			pvc.Status.Capacity[corev1.ResourceStorage] = resource.MustParse("2G")
			return pvc, nil
		}

		h := NewResizingHandler(diskService)

		_, err := h.Handle(context.Background(), vd)
		Expect(err).To(BeNil())

		resized, _ := service.GetCondition(vdcondition.ResizedType, vd.Status.Conditions)
		Expect(resized.Status).To(Equal(metav1.ConditionTrue))
		Expect(resized.Reason).To(Equal(vdcondition.Resized))
		Expect(resource.MustParse(vd.Status.Capacity)).To(Equal(pvc.Status.Capacity[corev1.ResourceStorage]))
	})
})
