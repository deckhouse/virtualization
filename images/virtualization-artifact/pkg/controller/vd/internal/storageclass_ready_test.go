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
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

var _ = Describe("StorageClassReadyHandler Run", func() {
	var (
		ctx context.Context
		vd  *virtv2.VirtualDisk
		pvc *corev1.PersistentVolumeClaim
		svc *StorageClassServiceMock
		sc  *storagev1.StorageClass
	)

	BeforeEach(func() {
		ctx = context.TODO()

		svc = &StorageClassServiceMock{
			GetPersistentVolumeClaimFunc: func(_ context.Context, _ *supplements.Generator) (*corev1.PersistentVolumeClaim, error) {
				return nil, nil
			},
		}

		sc = &storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "sc",
			},
		}

		vd = &virtv2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "vd",
				Generation: 1,
				UID:        "11111111-1111-1111-1111-111111111111",
			},
			Status: virtv2.VirtualDiskStatus{
				StorageClassName: sc.Name,
			},
		}

		supgen := supplements.NewGenerator(annotations.VDShortName, vd.Name, vd.Namespace, vd.UID)

		pvc = &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: supgen.PersistentVolumeClaim().Name,
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				StorageClassName: &sc.Name,
			},
		}
	})

	Context("PVC is already exists", func() {
		BeforeEach(func() {
			svc.GetPersistentVolumeClaimFunc = func(_ context.Context, _ *supplements.Generator) (*corev1.PersistentVolumeClaim, error) {
				return pvc, nil
			}
		})

		It("has existing StorageClass", func() {
			svc.GetStorageClassFunc = func(_ context.Context, _ string) (*storagev1.StorageClass, error) {
				return sc, nil
			}

			h := NewStorageClassReadyHandler(svc)
			res, err := h.Handle(ctx, vd)
			Expect(err).To(BeNil())
			Expect(res.IsZero()).To(BeTrue())
			Expect(vd.Status.StorageClassName).NotTo(BeEmpty())
			ExpectStorageClassReadyCondition(vd, metav1.ConditionTrue, vdcondition.StorageClassReady, false)
		})

		It("has terminating StorageClass", func() {
			svc.GetStorageClassFunc = func(_ context.Context, _ string) (*storagev1.StorageClass, error) {
				sc.DeletionTimestamp = ptr.To(metav1.Now())
				return sc, nil
			}

			h := NewStorageClassReadyHandler(svc)
			res, err := h.Handle(ctx, vd)
			Expect(err).To(BeNil())
			Expect(res.IsZero()).To(BeTrue())
			Expect(vd.Status.StorageClassName).NotTo(BeEmpty())
			ExpectStorageClassReadyCondition(vd, metav1.ConditionFalse, vdcondition.StorageClassNotReady, true)
		})

		It("has non-existing StorageClass", func() {
			svc.GetStorageClassFunc = func(_ context.Context, _ string) (*storagev1.StorageClass, error) {
				return nil, nil
			}

			h := NewStorageClassReadyHandler(svc)
			res, err := h.Handle(ctx, vd)
			Expect(err).To(BeNil())
			Expect(res.IsZero()).To(BeTrue())
			Expect(vd.Status.StorageClassName).NotTo(BeEmpty())
			ExpectStorageClassReadyCondition(vd, metav1.ConditionFalse, vdcondition.StorageClassNotReady, true)
		})
	})

	Context("StorageClass is specified in the spec of virtual disk", func() {
		BeforeEach(func() {
			vd.Spec.PersistentVolumeClaim.StorageClass = &sc.Name
		})

		It("has allowed StorageClass", func() {
			svc.IsStorageClassAllowedFunc = func(_ string) bool {
				return true
			}
			svc.GetStorageClassFunc = func(_ context.Context, _ string) (*storagev1.StorageClass, error) {
				return sc, nil
			}
			svc.IsStorageClassDeprecatedFunc = func(_ *storagev1.StorageClass) bool {
				return false
			}

			h := NewStorageClassReadyHandler(svc)
			res, err := h.Handle(ctx, vd)
			Expect(err).To(BeNil())
			Expect(res.IsZero()).To(BeTrue())
			Expect(vd.Status.StorageClassName).NotTo(BeEmpty())
			ExpectStorageClassReadyCondition(vd, metav1.ConditionTrue, vdcondition.StorageClassReady, false)
		})

		It("has not allowed StorageClass", func() {
			svc.IsStorageClassAllowedFunc = func(_ string) bool {
				return false
			}
			svc.IsStorageClassDeprecatedFunc = func(_ *storagev1.StorageClass) bool {
				return false
			}
			svc.GetStorageClassFunc = func(_ context.Context, _ string) (*storagev1.StorageClass, error) {
				return nil, nil
			}

			h := NewStorageClassReadyHandler(svc)
			res, err := h.Handle(ctx, vd)
			Expect(err).To(BeNil())
			Expect(res.IsZero()).To(BeTrue())
			Expect(vd.Status.StorageClassName).NotTo(BeEmpty())
			ExpectStorageClassReadyCondition(vd, metav1.ConditionFalse, vdcondition.StorageClassNotReady, true)
		})

		It("has terminating StorageClass", func() {
			svc.IsStorageClassAllowedFunc = func(_ string) bool {
				return true
			}
			svc.GetStorageClassFunc = func(_ context.Context, _ string) (*storagev1.StorageClass, error) {
				sc.DeletionTimestamp = ptr.To(metav1.Now())
				return sc, nil
			}
			svc.IsStorageClassDeprecatedFunc = func(_ *storagev1.StorageClass) bool {
				return false
			}

			h := NewStorageClassReadyHandler(svc)
			res, err := h.Handle(ctx, vd)
			Expect(err).To(BeNil())
			Expect(res.IsZero()).To(BeTrue())
			Expect(vd.Status.StorageClassName).NotTo(BeEmpty())
			ExpectStorageClassReadyCondition(vd, metav1.ConditionFalse, vdcondition.StorageClassNotReady, true)
		})

		It("has non-existing StorageClass", func() {
			svc.IsStorageClassAllowedFunc = func(_ string) bool {
				return true
			}
			svc.GetStorageClassFunc = func(_ context.Context, _ string) (*storagev1.StorageClass, error) {
				return nil, nil
			}
			svc.IsStorageClassDeprecatedFunc = func(_ *storagev1.StorageClass) bool {
				return false
			}

			h := NewStorageClassReadyHandler(svc)
			res, err := h.Handle(ctx, vd)
			Expect(err).To(BeNil())
			Expect(res.IsZero()).To(BeTrue())
			Expect(vd.Status.StorageClassName).NotTo(BeEmpty())
			ExpectStorageClassReadyCondition(vd, metav1.ConditionFalse, vdcondition.StorageClassNotReady, true)
		})
	})

	Context("StorageClass is specified in the module settings", func() {
		It("has existing StorageClass", func() {
			svc.GetModuleStorageClassFunc = func(_ context.Context) (*storagev1.StorageClass, error) {
				return sc, nil
			}
			svc.IsStorageClassDeprecatedFunc = func(_ *storagev1.StorageClass) bool {
				return false
			}

			h := NewStorageClassReadyHandler(svc)
			res, err := h.Handle(ctx, vd)
			Expect(err).To(BeNil())
			Expect(res.IsZero()).To(BeTrue())
			Expect(vd.Status.StorageClassName).NotTo(BeEmpty())
			ExpectStorageClassReadyCondition(vd, metav1.ConditionTrue, vdcondition.StorageClassReady, false)
		})

		It("has terminating StorageClass", func() {
			svc.GetModuleStorageClassFunc = func(_ context.Context) (*storagev1.StorageClass, error) {
				sc.DeletionTimestamp = ptr.To(metav1.Now())
				return sc, nil
			}
			svc.IsStorageClassDeprecatedFunc = func(_ *storagev1.StorageClass) bool {
				return false
			}

			h := NewStorageClassReadyHandler(svc)
			res, err := h.Handle(ctx, vd)
			Expect(err).To(BeNil())
			Expect(res.IsZero()).To(BeTrue())
			Expect(vd.Status.StorageClassName).NotTo(BeEmpty())
			ExpectStorageClassReadyCondition(vd, metav1.ConditionFalse, vdcondition.StorageClassNotReady, true)
		})
	})

	Context("Default StorageClass is specified in the cluster", func() {
		BeforeEach(func() {
			svc.GetModuleStorageClassFunc = func(_ context.Context) (*storagev1.StorageClass, error) {
				return nil, nil
			}
		})

		It("has existing StorageClass", func() {
			svc.GetDefaultStorageClassFunc = func(_ context.Context) (*storagev1.StorageClass, error) {
				return sc, nil
			}
			svc.IsStorageClassDeprecatedFunc = func(_ *storagev1.StorageClass) bool {
				return false
			}

			h := NewStorageClassReadyHandler(svc)
			res, err := h.Handle(ctx, vd)
			Expect(err).To(BeNil())
			Expect(res.IsZero()).To(BeTrue())
			Expect(vd.Status.StorageClassName).NotTo(BeEmpty())
			ExpectStorageClassReadyCondition(vd, metav1.ConditionTrue, vdcondition.StorageClassReady, false)
		})

		It("has terminating StorageClass", func() {
			svc.GetDefaultStorageClassFunc = func(_ context.Context) (*storagev1.StorageClass, error) {
				sc.DeletionTimestamp = ptr.To(metav1.Now())
				return sc, nil
			}
			svc.IsStorageClassDeprecatedFunc = func(_ *storagev1.StorageClass) bool {
				return false
			}

			h := NewStorageClassReadyHandler(svc)
			res, err := h.Handle(ctx, vd)
			Expect(err).To(BeNil())
			Expect(res.IsZero()).To(BeTrue())
			Expect(vd.Status.StorageClassName).NotTo(BeEmpty())
			ExpectStorageClassReadyCondition(vd, metav1.ConditionFalse, vdcondition.StorageClassNotReady, true)
		})
	})

	Context("Cannot determine StorageClass", func() {
		BeforeEach(func() {
			svc.GetModuleStorageClassFunc = func(_ context.Context) (*storagev1.StorageClass, error) {
				return nil, nil
			}
			svc.GetDefaultStorageClassFunc = func(_ context.Context) (*storagev1.StorageClass, error) {
				return nil, nil
			}
		})

		It("cannot find StorageClass", func() {
			h := NewStorageClassReadyHandler(svc)
			res, err := h.Handle(ctx, vd)
			Expect(err).To(BeNil())
			Expect(res.IsZero()).To(BeTrue())
			Expect(vd.Status.StorageClassName).To(BeEmpty())
			ExpectStorageClassReadyCondition(vd, metav1.ConditionFalse, vdcondition.StorageClassNotReady, true)
		})
	})
})

func ExpectStorageClassReadyCondition(vd *virtv2.VirtualDisk, status metav1.ConditionStatus, reason vdcondition.StorageClassReadyReason, msgExists bool) {
	ready, _ := conditions.GetCondition(vdcondition.StorageClassReadyType, vd.Status.Conditions)
	Expect(ready.Status).To(Equal(status))
	Expect(ready.Reason).To(Equal(reason.String()))
	Expect(ready.ObservedGeneration).To(Equal(vd.Generation))

	if msgExists {
		Expect(ready.Message).ToNot(BeEmpty())
	} else {
		Expect(ready.Message).To(BeEmpty())
	}
}
