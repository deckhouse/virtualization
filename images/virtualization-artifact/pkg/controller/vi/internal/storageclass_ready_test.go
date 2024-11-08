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

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

var _ = Describe("StorageClassHandler Run", func() {
	DescribeTable("Checking ActualStorageClass",
		func(storageClassFromModuleConfig string, vi *virtv2.VirtualImage, pvc *corev1.PersistentVolumeClaim, expectedStatus *string) {
			h := NewStorageClassReadyHandler(nil, storageClassFromModuleConfig)

			Expect(h.ActualStorageClass(vi, pvc)).To(Equal(expectedStatus))
		},
		Entry(
			"Should be nil if no VI",
			"",
			nil,
			nil,
			nil,
		),
		Entry(
			"Should be nil because no data",
			"",
			newVI(nil, "", ""),
			nil,
			nil,
		),
		Entry(
			"Should be from status if have in status, but not in spec",
			"",
			newVI(nil, "fromStatus", ""),
			nil,
			ptr.To("fromStatus"),
		),
		Entry(
			"Should be from status if have in status and in mc",
			"fromMC",
			newVI(nil, "fromStatus", ""),
			nil,
			ptr.To("fromStatus"),
		),
		Entry(
			"Should be from mc if only in mc",
			"fromMC",
			newVI(nil, "", ""),
			nil,
			ptr.To("fromMC"),
		),
		Entry(
			"Should be from spec if pvc does not exists",
			"fromMC",
			newVI(ptr.To("fromSPEC"), "fromStatus", ""),
			nil,
			ptr.To("fromSPEC"),
		),
		Entry(
			"Should be from spec if pvc exists, but his storage class is nil",
			"fromMC",
			newVI(ptr.To("fromSPEC"), "fromStatus", ""),
			newPVC(nil),
			ptr.To("fromSPEC"),
		),
		Entry(
			"Should be from spec if pvc exists, but his storage class is empty",
			"fromMC",
			newVI(ptr.To("fromSPEC"), "fromStatus", ""),
			newPVC(ptr.To("")),
			ptr.To("fromSPEC"),
		),
		Entry(
			"Should be from pvc if pvc exists",
			"fromMC",
			newVI(ptr.To("fromSPEC"), "fromStatus", ""),
			newPVC(ptr.To("fromPVC")),
			ptr.To("fromPVC"),
		),
	)

	DescribeTable("Checking returning conditions",
		func(diskServiceMock DiskService, vi *virtv2.VirtualImage, expectedStatus metav1.ConditionStatus, expectedReason vicondition.StorageClassReadyReason) {
			handler := NewStorageClassReadyHandler(diskServiceMock, "")
			_, err := handler.Handle(context.TODO(), vi)

			Expect(err).To(BeNil())
			condition, ok := service.GetCondition(vicondition.StorageClassReadyType, vi.Status.Conditions)
			Expect(ok).To(BeTrue())
			Expect(condition.Status).To(Equal(expectedStatus))
			Expect(condition.Reason).To(Equal(expectedReason))
		},
		Entry(
			"StorageClassReady must be false because used dvcr storage type",
			newDiskServiceMock(nil),
			newVI(nil, "", virtv2.StorageContainerRegistry),
			metav1.ConditionUnknown,
			vicondition.DVCRTypeUsed,
		),
		Entry(
			"StorageClassReady must be false because no storage class can be return",
			newDiskServiceMock(nil),
			newVI(nil, "", virtv2.StorageKubernetes),
			metav1.ConditionFalse,
			vicondition.StorageClassNotFound,
		),
		Entry(
			"StorageClassReady must be true because storage class from spec found",
			newDiskServiceMock(ptr.To("sc")),
			newVI(ptr.To("sc"), "", virtv2.StorageKubernetes),
			metav1.ConditionTrue,
			vicondition.StorageClassReady,
		),
		Entry(
			"StorageClassReady must be true because default storage class found",
			newDiskServiceMock(ptr.To("sc")),
			newVI(nil, "", virtv2.StorageKubernetes),
			metav1.ConditionTrue,
			vicondition.StorageClassReady,
		),
	)
})

func newDiskServiceMock(existedStorageClass *string) *DiskServiceMock {
	var diskServiceMock DiskServiceMock

	diskServiceMock.GetPersistentVolumeClaimFunc = func(ctx context.Context, sup *supplements.Generator) (*corev1.PersistentVolumeClaim, error) {
		return nil, nil
	}

	diskServiceMock.GetStorageClassFunc = func(ctx context.Context, storageClassName *string) (*storagev1.StorageClass, error) {
		switch {
		case existedStorageClass == nil:
			return nil, nil
		case storageClassName == nil:
			return &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: *existedStorageClass,
				},
			}, nil
		case *storageClassName == *existedStorageClass:
			return &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: *existedStorageClass,
				},
			}, nil
		default:
			return nil, nil
		}
	}

	return &diskServiceMock
}

func newVI(scInSpec *string, scInStatus string, storageType virtv2.StorageType) *virtv2.VirtualImage {
	return &virtv2.VirtualImage{
		Spec: virtv2.VirtualImageSpec{
			PersistentVolumeClaim: virtv2.VirtualImagePersistentVolumeClaim{
				StorageClass: scInSpec,
			},
			Storage: storageType,
		},
		Status: virtv2.VirtualImageStatus{
			StorageClassName: scInStatus,
		},
	}
}

func newPVC(sc *string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: sc,
		},
	}
}
