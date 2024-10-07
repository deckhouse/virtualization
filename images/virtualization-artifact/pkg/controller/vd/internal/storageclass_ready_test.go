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
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

var storageClasses []storev1.StorageClass

var _ = Describe("Storage class ready handler Run", func() {
	var vd *virtv2.VirtualDisk
	var diskService *DiskServiceMock
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
		vd.Status.Conditions[0].Status = metav1.ConditionUnknown
		vd.Spec.PersistentVolumeClaim.StorageClass = nil
		storageClasses = []storev1.StorageClass{}
	})

	It("Should false condition because storage class name is not provided", func() {
		_, err := handler.Handle(context.TODO(), vd)
		Expect(err).Should(BeNil())
		Expect(vd.Status.Conditions[0].Status).Should(Equal(metav1.ConditionFalse))
		Expect(vd.Status.Conditions[0].Reason).Should(Equal(vdcondition.StorageClassNameNotProvided))
	})

	It("Should true condition with correct data", func() {
		storageClasses = append(storageClasses, storev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
			},
		})
		vd.Spec.PersistentVolumeClaim.StorageClass = new(string)
		*vd.Spec.PersistentVolumeClaim.StorageClass = "test"

		_, err := handler.Handle(context.TODO(), vd)
		Expect(err).Should(BeNil())
		Expect(vd.Status.Conditions[0].Status).Should(Equal(metav1.ConditionTrue))
		Expect(vd.Status.Conditions[0].Reason).Should(Equal(vdcondition.StorageClassReady))
	})

	It("Should false condition because storage class not found", func() {
		vd.Spec.PersistentVolumeClaim.StorageClass = new(string)
		*vd.Spec.PersistentVolumeClaim.StorageClass = "test"

		_, err := handler.Handle(context.TODO(), vd)
		Expect(err).Should(BeNil())
		Expect(vd.Status.Conditions[0].Status).Should(Equal(metav1.ConditionFalse))
		Expect(vd.Status.Conditions[0].Reason).Should(Equal(vdcondition.StorageClassNotFound))
	})
})
