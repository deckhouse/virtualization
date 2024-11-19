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

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

var _ = Describe("StorageClassHandler Run", func() {
	DescribeTable("Checking returning conditions",
		func(args handlerTestArgs) {
			handler := NewStorageClassReadyHandler(args.DiskServiceMock)
			_, err := handler.Handle(context.TODO(), args.VI)

			Expect(err).To(BeNil())
			condition, ok := conditions.GetCondition(vicondition.StorageClassReadyType, args.VI.Status.Conditions)
			Expect(ok).To(BeTrue())
			Expect(condition.Status).To(Equal(args.ExpectedCondition.Status))
			Expect(condition.Reason).To(Equal(args.ExpectedCondition.Reason))
		},
		Entry(
			"StorageClassReady must be false because used DVCR storage type",
			handlerTestArgs{
				DiskServiceMock: newDiskServiceMock(nil),
				VI:              newVI(nil, virtv2.StorageContainerRegistry),
				ExpectedCondition: metav1.Condition{
					Status: metav1.ConditionUnknown,
					Reason: vicondition.DVCRTypeUsed.String(),
				},
			},
		),
		Entry(
			"StorageClassReady must be false because no storage class can be return",
			handlerTestArgs{
				DiskServiceMock: newDiskServiceMock(nil),
				VI:              newVI(nil, virtv2.StorageKubernetes),
				ExpectedCondition: metav1.Condition{
					Status: metav1.ConditionFalse,
					Reason: vicondition.StorageClassNotFound.String(),
				},
			},
		),
		Entry(
			"StorageClassReady must be true because storage class from spec found",
			handlerTestArgs{
				DiskServiceMock: newDiskServiceMock(ptr.To("sc")),
				VI:              newVI(ptr.To("sc"), virtv2.StorageKubernetes),
				ExpectedCondition: metav1.Condition{
					Status: metav1.ConditionTrue,
					Reason: vicondition.StorageClassReady.String(),
				},
			},
		),
		Entry(
			"StorageClassReady must be true because default storage class found",
			handlerTestArgs{
				DiskServiceMock: newDiskServiceMock(ptr.To("sc")),
				VI:              newVI(ptr.To("sc"), virtv2.StorageKubernetes),
				ExpectedCondition: metav1.Condition{
					Status: metav1.ConditionTrue,
					Reason: vicondition.StorageClassReady.String(),
				},
			},
		),
	)
})

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

func newVI(specSC *string, storageType virtv2.StorageType) *virtv2.VirtualImage {
	return &virtv2.VirtualImage{
		Spec: virtv2.VirtualImageSpec{
			PersistentVolumeClaim: virtv2.VirtualImagePersistentVolumeClaim{
				StorageClass: specSC,
			},
			Storage: storageType,
		},
		Status: virtv2.VirtualImageStatus{
			StorageClassName: "",
		},
	}
}
