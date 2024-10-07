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

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

var storageClasses []storev1.StorageClass

var _ = Describe("Storage class ready handler Run", func() {
	var vi *virtv2.VirtualImage
	var diskService *DiskServiceMock
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
			for _, storageClass := range storageClasses {
				if storageClass.Name == *storageClassName {
					return &storageClass, nil
				}
			}

			return nil, nil
		},
	}

	handler = NewStorageClassReadyHandler(diskService)

	AfterEach(func() {
		vi.Status.Conditions[0].Status = metav1.ConditionUnknown
		vi.Spec.PersistentVolumeClaim.StorageClass = nil
		vi.Spec.Storage = ""
		storageClasses = []storev1.StorageClass{}
	})

	It("Should false condition because storage class name is not provided", func() {
		vi.Spec.Storage = virtv2.StorageKubernetes

		_, err := handler.Handle(context.TODO(), vi)
		Expect(err).Should(BeNil())
		Expect(vi.Status.Conditions[0].Status).Should(Equal(metav1.ConditionFalse))
		Expect(vi.Status.Conditions[0].Reason).Should(Equal(vicondition.StorageClassNameNotProvided))
	})

	It("Should true condition because dvcr storage type used", func() {
		vi.Spec.Storage = virtv2.StorageContainerRegistry

		_, err := handler.Handle(context.TODO(), vi)
		Expect(err).Should(BeNil())
		Expect(vi.Status.Conditions[0].Status).Should(Equal(metav1.ConditionTrue))
		Expect(vi.Status.Conditions[0].Reason).Should(Equal(vicondition.DVCRTypeUsed))
	})

	It("Should true condition with correct data", func() {
		storageClasses = append(storageClasses, storev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
			},
		})
		vi.Spec.Storage = virtv2.StorageKubernetes
		vi.Spec.PersistentVolumeClaim.StorageClass = new(string)
		*vi.Spec.PersistentVolumeClaim.StorageClass = "test"

		_, err := handler.Handle(context.TODO(), vi)
		Expect(err).Should(BeNil())
		Expect(vi.Status.Conditions[0].Status).Should(Equal(metav1.ConditionTrue))
		Expect(vi.Status.Conditions[0].Reason).Should(Equal(vicondition.StorageClassReady))
	})

	It("Should false condition because storage class not found", func() {
		vi.Spec.Storage = virtv2.StorageKubernetes
		vi.Spec.PersistentVolumeClaim.StorageClass = new(string)
		*vi.Spec.PersistentVolumeClaim.StorageClass = "test"

		_, err := handler.Handle(context.TODO(), vi)
		Expect(err).Should(BeNil())
		Expect(vi.Status.Conditions[0].Status).Should(Equal(metav1.ConditionFalse))
		Expect(vi.Status.Conditions[0].Reason).Should(Equal(vicondition.StorageClassNotFound))
	})
})
