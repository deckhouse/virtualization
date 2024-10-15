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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

var (
	storageClasses []storev1.StorageClass
	cleanUpUsed    = false
)

var _ = Describe("Storage class ready handler Run", func() {
	var vi *virtv2.VirtualImage
	var diskService *DiskServiceMock
	var sourcesService *SourcesMock
	var handler *StorageClassReadyHandler

	vi = &virtv2.VirtualImage{
		Spec: virtv2.VirtualImageSpec{
			PersistentVolumeClaim: virtv2.VirtualImagePersistentVolumeClaim{},
		},
		Status: virtv2.VirtualImageStatus{
			Conditions: []metav1.Condition{
				{
					Type:   vicondition.StorageClassReadyType,
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
		CleanUpFunc: func(ctx context.Context, _ *virtv2.VirtualImage) (bool, error) {
			cleanUpUsed = true
			return true, nil
		},
	}

	handler = NewStorageClassReadyHandler(diskService, sourcesService)

	Context("DVCR type used", func() {
		It("Should pass validation because dvcr type used", func() {
			cleanup(vi)
			vi.Spec.Storage = virtv2.StorageContainerRegistry

			result, err := handler.Handle(context.TODO(), vi)
			Expect(err).Should(BeNil())
			Expect(result).Should(Equal(ctrl.Result{}))
			Expect(cleanUpUsed).Should(BeFalse())
			Expect(vi.Status.Conditions[0].Status).Should(Equal(metav1.ConditionTrue))
			Expect(vi.Status.Conditions[0].Reason).Should(Equal(vicondition.DVCRTypeUsed))
			Expect(vi.Status.Phase).ShouldNot(Equal(virtv2.ImagePending))
		})
	})

	Context("Kubernetes type used", func() {
		Context("Storage class specified", func() {
			Context("Storage class not found", func() {
				It("Should fail validation because Storage Class not found", func() {
					cleanup(vi)
					vi.Spec.Storage = virtv2.StorageKubernetes
					vi.Spec.PersistentVolumeClaim.StorageClass = new(string)
					*vi.Spec.PersistentVolumeClaim.StorageClass = "class1"

					result, err := handler.Handle(context.TODO(), vi)
					Expect(result).Should(Equal(reconcile.Result{}))
					Expect(err).Should(BeNil())
					Expect(cleanUpUsed).Should(BeTrue())
					Expect(vi.Status.Conditions[0].Status).Should(Equal(metav1.ConditionFalse))
					Expect(vi.Status.Conditions[0].Reason).Should(Equal(vicondition.StorageClassNotFound))
					Expect(vi.Status.Phase).Should(Equal(virtv2.ImagePending))
					Expect(vi.Status.StorageClassName).Should(Equal("class1"))
				})
			})

			Context("Storage class exists, first reconcile", func() {
				It("Should pass validation because correct storage class", func() {
					cleanup(vi)
					vi.Spec.Storage = virtv2.StorageKubernetes
					vi.Spec.PersistentVolumeClaim.StorageClass = new(string)
					*vi.Spec.PersistentVolumeClaim.StorageClass = "class1"
					storageClasses = append(storageClasses, storev1.StorageClass{
						ObjectMeta: metav1.ObjectMeta{
							Name: "class1",
						},
					})

					result, err := handler.Handle(context.TODO(), vi)
					Expect(result).Should(Equal(reconcile.Result{}))
					Expect(err).Should(BeNil())
					Expect(cleanUpUsed).Should(BeFalse())
					Expect(vi.Status.Conditions[0].Status).Should(Equal(metav1.ConditionTrue))
					Expect(vi.Status.Conditions[0].Reason).Should(Equal(vicondition.StorageClassReadyType))
					Expect(vi.Status.Phase).ShouldNot(Equal(virtv2.ImagePending))
					Expect(*vi.Spec.PersistentVolumeClaim.StorageClass).Should(Equal("class1"))
				})
			})

			Context("Storage class exists, second reconcile", func() {
				It("Should pass validation because correct storage class ", func() {
					cleanup(vi)
					vi.Spec.Storage = virtv2.StorageKubernetes
					vi.Spec.PersistentVolumeClaim.StorageClass = new(string)
					*vi.Spec.PersistentVolumeClaim.StorageClass = "class1"
					vi.Status.StorageClassName = "class1"
					storageClasses = append(storageClasses, storev1.StorageClass{
						ObjectMeta: metav1.ObjectMeta{
							Name: "class1",
						},
					})

					result, err := handler.Handle(context.TODO(), vi)
					Expect(result).Should(Equal(reconcile.Result{}))
					Expect(err).Should(BeNil())
					Expect(cleanUpUsed).Should(BeFalse())
					Expect(vi.Status.Conditions[0].Status).Should(Equal(metav1.ConditionTrue))
					Expect(vi.Status.Conditions[0].Reason).Should(Equal(vicondition.StorageClassReadyType))
					Expect(vi.Status.Phase).ShouldNot(Equal(virtv2.ImagePending))
					Expect(vi.Status.StorageClassName).Should(Equal("class1"))
				})
			})

			Context("Storage class changed from correct to incorrect", func() {
				It("Should fail validation because storage class is changed to incorrect", func() {
					cleanup(vi)
					vi.Spec.Storage = virtv2.StorageKubernetes
					vi.Spec.PersistentVolumeClaim.StorageClass = new(string)
					*vi.Spec.PersistentVolumeClaim.StorageClass = "class1"
					storageClasses = append(storageClasses, storev1.StorageClass{
						ObjectMeta: metav1.ObjectMeta{
							Name: "class1",
						},
					})

					_, err := handler.Handle(context.TODO(), vi)
					Expect(err).Should(BeNil())

					*vi.Spec.PersistentVolumeClaim.StorageClass = "class2"
					result, err := handler.Handle(context.TODO(), vi)

					Expect(result).Should(Equal(reconcile.Result{}))
					Expect(err).Should(BeNil())
					Expect(cleanUpUsed).Should(BeTrue())
					Expect(vi.Status.Conditions[0].Status).Should(Equal(metav1.ConditionFalse))
					Expect(vi.Status.Conditions[0].Reason).Should(Equal(vicondition.StorageClassNotFound))
					Expect(vi.Status.Phase).Should(Equal(virtv2.ImagePending))
					Expect(vi.Status.StorageClassName).Should(Equal("class2"))
				})
			})

			Context("Storage class changed from correct to incorrect", func() {
				It("Should fail validation because storage class is changed to incorrect", func() {
					cleanup(vi)
					vi.Spec.Storage = virtv2.StorageKubernetes
					vi.Spec.PersistentVolumeClaim.StorageClass = new(string)
					*vi.Spec.PersistentVolumeClaim.StorageClass = "class1"
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

					_, err := handler.Handle(context.TODO(), vi)
					Expect(err).Should(BeNil())

					*vi.Spec.PersistentVolumeClaim.StorageClass = "class2"
					result, err := handler.Handle(context.TODO(), vi)

					Expect(err).Should(BeNil())
					Expect(result).Should(Equal(reconcile.Result{}))
					Expect(cleanUpUsed).Should(BeTrue())
					Expect(vi.Status.Conditions[0].Status).Should(Equal(metav1.ConditionTrue))
					Expect(vi.Status.Conditions[0].Reason).Should(Equal(vicondition.StorageClassReady))
					Expect(vi.Status.Phase).ShouldNot(Equal(virtv2.ImagePending))
					Expect(vi.Status.StorageClassName).Should(Equal(*vi.Spec.PersistentVolumeClaim.StorageClass))
				})
			})
		})

		Context("Storage class not specified", func() {
			Context("No default class, empty status", func() {
				It("Should fail validation because Default Storage Class not found", func() {
					cleanup(vi)
					result, err := handler.Handle(context.TODO(), vi)
					Expect(err).Should(BeNil())
					Expect(result).Should(Equal(reconcile.Result{}))
					Expect(cleanUpUsed).Should(BeTrue())
					Expect(vi.Status.Conditions[0].Status).Should(Equal(metav1.ConditionFalse))
					Expect(vi.Status.Conditions[0].Reason).Should(Equal(vicondition.StorageClassNameNotProvided))
					Expect(vi.Status.Phase).Should(Equal(virtv2.ImagePending))
					Expect(vi.Status.StorageClassName).Should(BeEmpty())
				})
			})

			Context("Existed default class, empty status", func() {
				It("Should pass validation because Default Storage Class exist", func() {
					cleanup(vi)
					storageClasses = append(storageClasses, storev1.StorageClass{
						ObjectMeta: metav1.ObjectMeta{
							Name: "class1",
							Annotations: map[string]string{
								common.AnnDefaultStorageClass: "true",
							},
						},
					})

					result, err := handler.Handle(context.TODO(), vi)
					Expect(result).Should(Equal(reconcile.Result{}))
					Expect(err).Should(BeNil())
					Expect(cleanUpUsed).Should(BeFalse())
					Expect(vi.Status.Conditions[0].Status).Should(Equal(metav1.ConditionTrue))
					Expect(vi.Status.Conditions[0].Reason).Should(Equal(vicondition.StorageClassReady))
					Expect(vi.Status.Phase).ShouldNot(Equal(virtv2.ImagePending))
					Expect(vi.Status.StorageClassName).Should(Equal("class1"))
				})
			})

			Context("Has incorrect storage class in status", func() {
				It("Should requeue", func() {
					cleanup(vi)
					vi.Status.StorageClassName = "class1"

					result, err := handler.Handle(context.TODO(), vi)
					Expect(err).Should(BeNil())
					Expect(result).Should(Equal(reconcile.Result{Requeue: true}))
					Expect(cleanUpUsed).Should(BeTrue())
					Expect(vi.Status.Conditions[0].Status).Should(Equal(metav1.ConditionFalse))
					Expect(vi.Status.Conditions[0].Reason).Should(Equal(vicondition.StorageClassNameNotProvided))
					Expect(vi.Status.Phase).Should(Equal(virtv2.ImagePending))
					Expect(vi.Status.StorageClassName).Should(Equal(""))
				})
			})

			Context("Has correct storage class in status", func() {
				It("Should pass validation because Default Storage Class exist", func() {
					cleanup(vi)
					vi.Status.StorageClassName = "class1"
					storageClasses = append(storageClasses, storev1.StorageClass{
						ObjectMeta: metav1.ObjectMeta{
							Name: "class1",
						},
					})

					result, err := handler.Handle(context.TODO(), vi)
					Expect(err).Should(BeNil())
					Expect(result).Should(Equal(reconcile.Result{}))
					Expect(cleanUpUsed).Should(BeFalse())
					Expect(vi.Status.Conditions[0].Status).Should(Equal(metav1.ConditionTrue))
					Expect(vi.Status.Conditions[0].Reason).Should(Equal(vicondition.StorageClassReadyType))
					Expect(vi.Status.Phase).ShouldNot(Equal(virtv2.ImagePending))
					Expect(vi.Status.StorageClassName).Should(Equal("class1"))
				})
			})
		})
	})
})

func cleanup(vi *virtv2.VirtualImage) {
	vi.Status.Phase = ""
	vi.Status.Conditions[0].Status = metav1.ConditionUnknown
	vi.Status.StorageClassName = ""
	vi.Spec.PersistentVolumeClaim.StorageClass = nil
	cleanUpUsed = false
	storageClasses = []storev1.StorageClass{}
}
