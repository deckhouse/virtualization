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
	storev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

var storageClasses []storev1.StorageClass
var cleanUpUsed bool = false

var _ = Describe("Storage class ready handler Run", func() {
	var vd *virtv2.VirtualDisk
	var diskService *DiskServiceMock
	var sourcesService *SourcesMock
	var handler *StorageClassReadyHandler

	vd = &virtv2.VirtualDisk{
		Spec: virtv2.VirtualDiskSpec{
			PersistentVolumeClaim: virtv2.VirtualDiskPersistentVolumeClaim{},
		},
		Status: virtv2.VirtualDiskStatus{
			Conditions: []metav1.Condition{
				{
					Type:   vdcondition.StorageClassReadyType,
					Status: metav1.ConditionUnknown,
				},
			},
		},
	}

	diskService = &DiskServiceMock{
		GetStorageClassFunc: func(ctx context.Context, storageClassName *string) (*storev1.StorageClass, error) {
			if storageClassName != nil {
				for _, storageClass := range storageClasses {
					if storageClass.Name == *storageClassName {
						return &storageClass, nil
					}
				}
			}

			for _, storageClass := range storageClasses {
				isDefault, ok := storageClass.Annotations[common.AnnDefaultStorageClass]

				if ok && isDefault == "true" {
					return &storageClass, nil
				}
			}

			return nil, nil
		},
	}

	sourcesService = &SourcesMock{
		CleanUpFunc: func(ctx context.Context, _ *virtv2.VirtualDisk) (bool, error) {
			cleanUpUsed = true
			return true, nil
		},
	}

	handler = NewStorageClassReadyHandler(diskService, sourcesService)

	Context("Storage class specified", func() {
		Context("Storage class not found", func() {
			It("Should fail validation because Storage Class not found", func() {
				cleanup(vd)
				vd.Spec.PersistentVolumeClaim.StorageClass = new(string)
				*vd.Spec.PersistentVolumeClaim.StorageClass = "class1"

				result, err := handler.Handle(context.TODO(), vd)
				Expect(result).Should(Equal(reconcile.Result{}))
				Expect(err).Should(BeNil())
				Expect(cleanUpUsed).Should(BeTrue())
				Expect(vd.Status.Conditions[0].Status).Should(Equal(metav1.ConditionFalse))
				Expect(vd.Status.Conditions[0].Reason).Should(Equal(vdcondition.StorageClassNotFound))
				Expect(vd.Status.Phase).Should(Equal(virtv2.DiskPending))
				Expect(vd.Status.StorageClassName).Should(Equal("class1"))
			})
		})

		Context("Storage class exists, first reconcile", func() {
			It("Should pass validation because correct storage class", func() {
				cleanup(vd)
				vd.Spec.PersistentVolumeClaim.StorageClass = new(string)
				*vd.Spec.PersistentVolumeClaim.StorageClass = "class1"
				storageClasses = append(storageClasses, storev1.StorageClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "class1",
					},
				})

				result, err := handler.Handle(context.TODO(), vd)
				Expect(result).Should(Equal(reconcile.Result{}))
				Expect(err).Should(BeNil())
				Expect(cleanUpUsed).Should(BeFalse())
				Expect(vd.Status.Conditions[0].Status).Should(Equal(metav1.ConditionTrue))
				Expect(vd.Status.Conditions[0].Reason).Should(Equal(vdcondition.StorageClassReadyType))
				Expect(vd.Status.Phase).ShouldNot(Equal(virtv2.DiskPending))
				Expect(*vd.Spec.PersistentVolumeClaim.StorageClass).Should(Equal("class1"))
			})
		})

		Context("Storage class exists, second reconcile", func() {
			It("Should pass validation because correct storage class ", func() {
				cleanup(vd)
				vd.Spec.PersistentVolumeClaim.StorageClass = new(string)
				*vd.Spec.PersistentVolumeClaim.StorageClass = "class1"
				vd.Status.StorageClassName = "class1"
				storageClasses = append(storageClasses, storev1.StorageClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "class1",
					},
				})

				result, err := handler.Handle(context.TODO(), vd)
				Expect(result).Should(Equal(reconcile.Result{}))
				Expect(err).Should(BeNil())
				Expect(cleanUpUsed).Should(BeFalse())
				Expect(vd.Status.Conditions[0].Status).Should(Equal(metav1.ConditionTrue))
				Expect(vd.Status.Conditions[0].Reason).Should(Equal(vdcondition.StorageClassReadyType))
				Expect(vd.Status.Phase).ShouldNot(Equal(virtv2.DiskPending))
				Expect(vd.Status.StorageClassName).Should(Equal("class1"))
			})
		})

		Context("Storage class changed from correct to incorrect", func() {
			It("Should fail validation because storage class is changed to incorrect", func() {
				cleanup(vd)
				vd.Spec.PersistentVolumeClaim.StorageClass = new(string)
				*vd.Spec.PersistentVolumeClaim.StorageClass = "class1"
				storageClasses = append(storageClasses, storev1.StorageClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "class1",
					},
				})

				_, err := handler.Handle(context.TODO(), vd)
				Expect(err).Should(BeNil())

				*vd.Spec.PersistentVolumeClaim.StorageClass = "class2"
				result, err := handler.Handle(context.TODO(), vd)

				Expect(result).Should(Equal(reconcile.Result{}))
				Expect(err).Should(BeNil())
				Expect(cleanUpUsed).Should(BeTrue())
				Expect(vd.Status.Conditions[0].Status).Should(Equal(metav1.ConditionFalse))
				Expect(vd.Status.Conditions[0].Reason).Should(Equal(vdcondition.StorageClassNotFound))
				Expect(vd.Status.Phase).Should(Equal(virtv2.DiskPending))
				Expect(vd.Status.StorageClassName).Should(Equal("class2"))
			})
		})

		Context("Storage class changed from correct to incorrect", func() {
			It("Should fail validation because storage class is changed to incorrect", func() {
				cleanup(vd)
				vd.Spec.PersistentVolumeClaim.StorageClass = new(string)
				*vd.Spec.PersistentVolumeClaim.StorageClass = "class1"
				storageClasses = append(storageClasses, storev1.StorageClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "class1",
					},
				})
				storageClasses = append(storageClasses, storev1.StorageClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "class2",
					},
				})

				_, err := handler.Handle(context.TODO(), vd)
				Expect(err).Should(BeNil())

				*vd.Spec.PersistentVolumeClaim.StorageClass = "class2"
				result, err := handler.Handle(context.TODO(), vd)

				Expect(err).Should(BeNil())
				Expect(result).Should(Equal(reconcile.Result{}))
				Expect(cleanUpUsed).Should(BeTrue())
				Expect(vd.Status.Conditions[0].Status).Should(Equal(metav1.ConditionTrue))
				Expect(vd.Status.Conditions[0].Reason).Should(Equal(vdcondition.StorageClassReady))
				Expect(vd.Status.Phase).ShouldNot(Equal(virtv2.DiskPending))
				Expect(vd.Status.StorageClassName).Should(Equal(*vd.Spec.PersistentVolumeClaim.StorageClass))
			})
		})
	})

	Context("Storage class not specified", func() {
		Context("No default class, empty status", func() {
			It("Should fail validation because Default Storage Class not found", func() {
				cleanup(vd)
				result, err := handler.Handle(context.TODO(), vd)
				Expect(err).Should(BeNil())
				Expect(result).Should(Equal(reconcile.Result{}))
				Expect(cleanUpUsed).Should(BeTrue())
				Expect(vd.Status.Conditions[0].Status).Should(Equal(metav1.ConditionFalse))
				Expect(vd.Status.Conditions[0].Reason).Should(Equal(vdcondition.StorageClassNameNotProvided))
				Expect(vd.Status.Phase).Should(Equal(virtv2.DiskPending))
				Expect(vd.Status.StorageClassName).Should(BeEmpty())
			})
		})

		Context("Existed default class, empty status", func() {
			It("Should pass validation because Default Storage Class exist", func() {
				cleanup(vd)
				storageClasses = append(storageClasses, storev1.StorageClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "class1",
						Annotations: map[string]string{
							common.AnnDefaultStorageClass: "true",
						},
					},
				})

				result, err := handler.Handle(context.TODO(), vd)
				Expect(result).Should(Equal(reconcile.Result{}))
				Expect(err).Should(BeNil())
				Expect(cleanUpUsed).Should(BeFalse())
				Expect(vd.Status.Conditions[0].Status).Should(Equal(metav1.ConditionTrue))
				Expect(vd.Status.Conditions[0].Reason).Should(Equal(vdcondition.StorageClassReady))
				Expect(vd.Status.Phase).ShouldNot(Equal(virtv2.DiskPending))
				Expect(vd.Status.StorageClassName).Should(Equal("class1"))
			})
		})

		Context("Has incorrect storage class in status", func() {
			It("Should requeue", func() {
				cleanup(vd)
				vd.Status.StorageClassName = "class1"

				result, err := handler.Handle(context.TODO(), vd)
				Expect(err).Should(BeNil())
				Expect(result).Should(Equal(reconcile.Result{Requeue: true}))
				Expect(cleanUpUsed).Should(BeTrue())
				Expect(vd.Status.Conditions[0].Status).Should(Equal(metav1.ConditionFalse))
				Expect(vd.Status.Conditions[0].Reason).Should(Equal(vdcondition.StorageClassNameNotProvided))
				Expect(vd.Status.Phase).Should(Equal(virtv2.DiskPending))
				Expect(vd.Status.StorageClassName).Should(Equal(""))
			})
		})

		Context("Has correct storage class in status", func() {
			It("Should pass validation because Default Storage Class exist", func() {
				cleanup(vd)
				vd.Status.StorageClassName = "class1"
				storageClasses = append(storageClasses, storev1.StorageClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "class1",
					},
				})

				result, err := handler.Handle(context.TODO(), vd)
				Expect(err).Should(BeNil())
				Expect(result).Should(Equal(reconcile.Result{}))
				Expect(cleanUpUsed).Should(BeFalse())
				Expect(vd.Status.Conditions[0].Status).Should(Equal(metav1.ConditionTrue))
				Expect(vd.Status.Conditions[0].Reason).Should(Equal(vdcondition.StorageClassReadyType))
				Expect(vd.Status.Phase).ShouldNot(Equal(virtv2.DiskPending))
				Expect(vd.Status.StorageClassName).Should(Equal("class1"))
			})
		})
	})
})

func cleanup(vd *virtv2.VirtualDisk) {
	vd.Status.Phase = ""
	vd.Status.Conditions[0].Status = metav1.ConditionUnknown
	vd.Status.StorageClassName = ""
	vd.Spec.PersistentVolumeClaim.StorageClass = nil
	cleanUpUsed = false
	storageClasses = []storev1.StorageClass{}
}
