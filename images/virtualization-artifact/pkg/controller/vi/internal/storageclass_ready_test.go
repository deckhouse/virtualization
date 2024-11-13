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
		func(args actualScNameTestArgs) {
			h := NewStorageClassReadyHandler(nil, args.StorageClassFromModuleConfig)

			Expect(h.ActualStorageClass(args.VI, args.PVC)).To(Equal(args.ExpectedStatus))
		},
		Entry(
			"Should be nil if no VI",
			actualScNameTestArgs{
				StorageClassFromModuleConfig: "",
				VI:                           nil,
				PVC:                          nil,
				ExpectedStatus:               nil,
			},
		),
		Entry(
			"Should be nil because no data",
			actualScNameTestArgs{
				StorageClassFromModuleConfig: "",
				VI:                           newVI(nil, "", ""),
				PVC:                          nil,
				ExpectedStatus:               nil,
			},
		),
		Entry(
			"Should be from status if have in status, but not in spec",
			actualScNameTestArgs{
				StorageClassFromModuleConfig: "",
				VI:                           newVI(nil, "fromStatus", ""),
				PVC:                          nil,
				ExpectedStatus:               ptr.To("fromStatus"),
			},
		),
		Entry(
			"Should be from status if have in status and in mc",
			actualScNameTestArgs{
				StorageClassFromModuleConfig: "fromMC",
				VI:                           newVI(nil, "fromStatus", ""),
				PVC:                          nil,
				ExpectedStatus:               ptr.To("fromStatus"),
			},
		),
		Entry(
			"Should be from mc if only in mc",
			actualScNameTestArgs{
				StorageClassFromModuleConfig: "fromMC",
				VI:                           newVI(nil, "", ""),
				PVC:                          nil,
				ExpectedStatus:               ptr.To("fromMC"),
			},
		),
		Entry(
			"Should be from spec if pvc does not exists",
			actualScNameTestArgs{
				StorageClassFromModuleConfig: "fromMC",
				VI:                           newVI(ptr.To("fromSpec"), "fromStatus", ""),
				PVC:                          nil,
				ExpectedStatus:               ptr.To("fromSpec"),
			},
		),
		Entry(
			"Should be from spec if pvc exists, but his storage class is nil",
			actualScNameTestArgs{
				StorageClassFromModuleConfig: "fromMC",
				VI:                           newVI(ptr.To("fromSpec"), "fromStatus", ""),
				PVC:                          newPVC(nil),
				ExpectedStatus:               ptr.To("fromSpec"),
			},
		),
		Entry(
			"Should be from spec if pvc exists, but his storage class is empty",
			actualScNameTestArgs{
				StorageClassFromModuleConfig: "fromMC",
				VI:                           newVI(ptr.To("fromSpec"), "fromStatus", ""),
				PVC:                          newPVC(ptr.To("")),
				ExpectedStatus:               ptr.To("fromSpec"),
			},
		),
		Entry(
			"Should be from pvc if pvc exists",
			actualScNameTestArgs{
				StorageClassFromModuleConfig: "fromMC",
				VI:                           newVI(ptr.To("fromSpec"), "fromStatus", ""),
				PVC:                          newPVC(ptr.To("fromPVC")),
				ExpectedStatus:               ptr.To("fromPVC"),
			},
		),
	)

	DescribeTable("Checking returning conditions",
		func(args handlerTestArgs) {
			handler := NewStorageClassReadyHandler(args.DiskServiceMock, "")
			_, err := handler.Handle(context.TODO(), args.VI)

			Expect(err).To(BeNil())
			condition, ok := service.GetCondition(vicondition.StorageClassReadyType, args.VI.Status.Conditions)
			Expect(ok).To(BeTrue())
			Expect(condition.Status).To(Equal(args.ExpectedCondition.Status))
			Expect(condition.Reason).To(Equal(args.ExpectedCondition.Reason))
		},
		Entry(
			"StorageClassReady must be false because used dvcr storage type",
			handlerTestArgs{
				DiskServiceMock: newDiskServiceMock(nil),
				VI:              newVI(nil, "", virtv2.StorageContainerRegistry),
				ExpectedCondition: metav1.Condition{
					Status: metav1.ConditionUnknown,
					Reason: vicondition.DVCRTypeUsed,
				},
			},
		),
		Entry(
			"StorageClassReady must be false because no storage class can be return",
			handlerTestArgs{
				DiskServiceMock: newDiskServiceMock(nil),
				VI:              newVI(nil, "", virtv2.StorageKubernetes),
				ExpectedCondition: metav1.Condition{
					Status: metav1.ConditionFalse,
					Reason: vicondition.StorageClassNotFound,
				},
			},
		),
		Entry(
			"StorageClassReady must be true because storage class from spec found",
			handlerTestArgs{
				DiskServiceMock: newDiskServiceMock(ptr.To("sc")),
				VI:              newVI(ptr.To("sc"), "", virtv2.StorageKubernetes),
				ExpectedCondition: metav1.Condition{
					Status: metav1.ConditionTrue,
					Reason: vicondition.StorageClassReady,
				},
			},
		),
		Entry(
			"StorageClassReady must be true because default storage class found",
			handlerTestArgs{
				DiskServiceMock: newDiskServiceMock(ptr.To("sc")),
				VI:              newVI(ptr.To("sc"), "", virtv2.StorageKubernetes),
				ExpectedCondition: metav1.Condition{
					Status: metav1.ConditionTrue,
					Reason: vicondition.StorageClassReady,
				},
			},
		),
	)
})

type actualScNameTestArgs struct {
	StorageClassFromModuleConfig string
	VI                           *virtv2.VirtualImage
	PVC                          *corev1.PersistentVolumeClaim
	ExpectedStatus               *string
}

type handlerTestArgs struct {
	DiskServiceMock   DiskService
	VI                *virtv2.VirtualImage
	ExpectedCondition metav1.Condition
}

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
