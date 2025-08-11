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
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

var _ = Describe("StorageClassHandler Run", func() {
	Describe("Check for the storage ContainerRegistry", func() {
		var vi *virtv2.VirtualImage

		BeforeEach(func() {
			vi = newVI(nil, virtv2.StorageContainerRegistry)
		})

		It("doest not have StorageClass", func() {
			handler := NewStorageClassReadyHandler(nil, nil)

			res, err := handler.Handle(context.TODO(), vi)
			Expect(err).To(BeNil())
			Expect(res.IsZero()).To(BeTrue())

			_, ok := conditions.GetCondition(vicondition.StorageClassReadyType, vi.Status.Conditions)
			Expect(ok).To(BeFalse())
			Expect(vi.Status.StorageClassName).To(BeEmpty())
		})
	})

	DescribeTable("Check for the storage PersistentVolumeClaim",
		func(args handlerTestArgs) {
			recorder := &eventrecord.EventRecorderLoggerMock{
				EventFunc: func(_ client.Object, _, _, _ string) {},
			}
			handler := NewStorageClassReadyHandler(recorder, args.StorageClassServiceMock)
			_, err := handler.Handle(context.TODO(), args.VI)

			Expect(err).To(BeNil())
			condition, ok := conditions.GetCondition(vicondition.StorageClassReadyType, args.VI.Status.Conditions)
			Expect(ok).To(BeTrue())
			Expect(condition.Status).To(Equal(args.ExpectedCondition.Status))
			Expect(condition.Reason).To(Equal(args.ExpectedCondition.Reason))
		},
		Entry(
			"StorageClassReady must be false because no storage class can be return",
			handlerTestArgs{
				StorageClassServiceMock: newStorageClassServiceMock(nil, false),
				VI:                      newVI(nil, virtv2.StoragePersistentVolumeClaim),
				ExpectedCondition: metav1.Condition{
					Status: metav1.ConditionFalse,
					Reason: vicondition.StorageClassNotFound.String(),
				},
			},
		),
		Entry(
			"StorageClassReady must be true because storage class from spec found",
			handlerTestArgs{
				StorageClassServiceMock: newStorageClassServiceMock(ptr.To("sc"), false),
				VI:                      newVI(ptr.To("sc"), virtv2.StoragePersistentVolumeClaim),
				ExpectedCondition: metav1.Condition{
					Status: metav1.ConditionTrue,
					Reason: vicondition.StorageClassReady.String(),
				},
			},
		),
		Entry(
			"StorageClassReady must be true because default storage class found",
			handlerTestArgs{
				StorageClassServiceMock: newStorageClassServiceMock(ptr.To("sc"), false),
				VI:                      newVI(ptr.To("sc"), virtv2.StoragePersistentVolumeClaim),
				ExpectedCondition: metav1.Condition{
					Status: metav1.ConditionTrue,
					Reason: vicondition.StorageClassReady.String(),
				},
			},
		),
		Entry(
			"StorageClassReady must be false because storage class is not supported",
			handlerTestArgs{
				StorageClassServiceMock: newStorageClassServiceMock(ptr.To("sc"), true),
				VI:                      newVI(ptr.To("sc"), virtv2.StoragePersistentVolumeClaim),
				ExpectedCondition: metav1.Condition{
					Status: metav1.ConditionFalse,
					Reason: vicondition.StorageClassNotReady.String(),
				},
			},
		),
	)
})

type handlerTestArgs struct {
	StorageClassServiceMock *StorageClassServiceMock
	VI                      *virtv2.VirtualImage
	ExpectedCondition       metav1.Condition
}

func newStorageClassServiceMock(existedStorageClass *string, unsupportedStorageClass bool) *StorageClassServiceMock {
	var storageClassServiceMock StorageClassServiceMock

	storageClassServiceMock.GetPersistentVolumeClaimFunc = func(ctx context.Context, sup *supplements.Generator) (*corev1.PersistentVolumeClaim, error) {
		return nil, nil
	}

	storageClassServiceMock.IsStorageClassDeprecatedFunc = func(_ *storagev1.StorageClass) bool {
		return false
	}

	storageClassServiceMock.GetStorageClassFunc = func(ctx context.Context, storageClassName string) (*storagev1.StorageClass, error) {
		switch {
		case existedStorageClass == nil:
			return nil, nil
		case storageClassName == "":
			return &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: *existedStorageClass,
				},
			}, nil
		case storageClassName == *existedStorageClass:
			return &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: *existedStorageClass,
				},
			}, nil
		default:
			return nil, nil
		}
	}

	storageClassServiceMock.GetStorageProfileFunc = func(ctx context.Context, name string) (*cdiv1.StorageProfile, error) {
		return &cdiv1.StorageProfile{}, nil
	}

	storageClassServiceMock.GetModuleStorageClassFunc = func(ctx context.Context) (*storagev1.StorageClass, error) {
		return nil, service.ErrDefaultStorageClassNotFound
	}

	storageClassServiceMock.GetDefaultStorageClassFunc = func(ctx context.Context) (*storagev1.StorageClass, error) {
		return nil, service.ErrDefaultStorageClassNotFound
	}

	storageClassServiceMock.IsStorageClassAllowedFunc = func(_ string) bool {
		return true
	}

	storageClassServiceMock.ValidateClaimPropertySetsFunc = func(_ *cdiv1.StorageProfile) error {
		if unsupportedStorageClass {
			return fmt.Errorf(
				"the storage class %q lacks of capabilities to support 'Virtual Images on PVC' function; use StorageClass that supports volume mode 'Block' and access mode 'ReadWriteMany'",
				*existedStorageClass,
			)
		}
		return nil
	}

	return &storageClassServiceMock
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
