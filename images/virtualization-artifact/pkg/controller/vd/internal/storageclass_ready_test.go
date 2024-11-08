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
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

var _ = Describe("StorageClassHandler Run", func() {
	DescribeTable("Check StorageClass handler",
		func(
			vd *virtv2.VirtualDisk,
			existedSc, scInPVC *string,
			expectedConditionStatus metav1.ConditionStatus,
			expectedConditionReason string,
			expectedScInStatus string,
		) {
			handler := NewStorageClassReadyHandler(newDiskServiceMock(existedSc, scInPVC))
			_, err := handler.Handle(context.TODO(), vd)

			Expect(err).To(BeNil())
			condition, ok := service.GetCondition(vdcondition.StorageClassReadyType, vd.Status.Conditions)
			Expect(ok).To(BeTrue())
			Expect(condition.Status).To(Equal(expectedConditionStatus))
			Expect(condition.Reason).To(Equal(expectedConditionReason))
			Expect(vd.Status.StorageClassName).To(Equal(expectedScInStatus))
		},
		Entry(
			"Should be false condition and empty sc in status",
			newVD(nil, ""),
			nil,
			nil,
			metav1.ConditionFalse,
			vdcondition.StorageClassNotFound,
			"",
		),
		Entry(
			"Should be true condition because PVC exists",
			newVD(nil, ""),
			ptr.To("sc"),
			ptr.To("sc"),
			metav1.ConditionTrue,
			vdcondition.StorageClassReady,
			"sc",
		),
		Entry(
			"Should be true condition because sc in spec",
			newVD(ptr.To("sc"), ""),
			ptr.To("sc"),
			nil,
			metav1.ConditionTrue,
			vdcondition.StorageClassReady,
			"sc",
		),
		Entry(
			"Should be true condition because has default sc",
			newVD(nil, ""),
			ptr.To("sc"),
			nil,
			metav1.ConditionTrue,
			vdcondition.StorageClassReady,
			"sc",
		),
		Entry(
			"Should be false condition because sc from status not found",
			newVD(nil, "status"),
			ptr.To("sc"),
			nil,
			metav1.ConditionFalse,
			vdcondition.StorageClassNotFound,
			"status",
		),
		Entry(
			"Should be pvc in status",
			newVD(ptr.To("spec"), "status"),
			ptr.To("pvc"),
			ptr.To("pvc"),
			metav1.ConditionTrue,
			vdcondition.StorageClassReady,
			"pvc",
		),
		Entry(
			"Should be pvc in status",
			newVD(ptr.To("spec"), "status"),
			ptr.To("pvc"),
			ptr.To("pvc"),
			metav1.ConditionTrue,
			vdcondition.StorageClassReady,
			"pvc",
		),
		Entry(
			"Should be spec in status",
			newVD(ptr.To("spec"), "status"),
			ptr.To("spec"),
			nil,
			metav1.ConditionTrue,
			vdcondition.StorageClassReady,
			"spec",
		),
	)
})

func newDiskServiceMock(existedStorageClass, scInPVC *string) *DiskServiceMock {
	var diskServiceMock DiskServiceMock

	diskServiceMock.GetPersistentVolumeClaimFunc = func(_ context.Context, _ *supplements.Generator) (*corev1.PersistentVolumeClaim, error) {
		return newPVC(scInPVC), nil
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

func newVD(scInSpec *string, scInStatus string) *virtv2.VirtualDisk {
	return &virtv2.VirtualDisk{
		Spec: virtv2.VirtualDiskSpec{
			PersistentVolumeClaim: virtv2.VirtualDiskPersistentVolumeClaim{
				StorageClass: scInSpec,
			},
		},
		Status: virtv2.VirtualDiskStatus{
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
