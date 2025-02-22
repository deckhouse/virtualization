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
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

var _ = Describe("StorageClassHandler Run", func() {
	DescribeTable("Check StorageClass handler",
		func(args handlerTestArgs) {
			recorder := &eventrecord.EventRecorderLoggerMock{
				EventFunc: func(_ client.Object, _, _, _ string) {},
			}
			handler := NewStorageClassReadyHandler(recorder, newDiskServiceMock(args.StorageClassExistedInCluster, args.StorageClassInExistedPVC))
			_, err := handler.Handle(context.TODO(), args.VirtualDisk)

			Expect(err).To(BeNil())
			condition, ok := conditions.GetCondition(vdcondition.StorageClassReadyType, args.VirtualDisk.Status.Conditions)
			Expect(ok).To(BeTrue())
			Expect(condition.Status).To(Equal(args.ExpectedCondition.Status))
			Expect(condition.Reason).To(Equal(args.ExpectedCondition.Reason))
			Expect(args.VirtualDisk.Status.StorageClassName).To(Equal(args.ExpectedStorageClassInStatus))
		},
		Entry(
			"Should be false condition and empty sc in status",
			handlerTestArgs{
				VirtualDisk:                  newVD(nil, ""),
				StorageClassExistedInCluster: nil,
				StorageClassInExistedPVC:     nil,
				ExpectedCondition: metav1.Condition{
					Status: metav1.ConditionFalse,
					Reason: vdcondition.StorageClassNotFound.String(),
				},
				ExpectedStorageClassInStatus: "",
			},
		),
		Entry(
			"Should be \"true\" status condition because PVC exists",
			handlerTestArgs{
				VirtualDisk:                  newVD(nil, ""),
				StorageClassExistedInCluster: ptr.To("sc"),
				StorageClassInExistedPVC:     ptr.To("sc"),
				ExpectedCondition: metav1.Condition{
					Status: metav1.ConditionTrue,
					Reason: vdcondition.StorageClassReady.String(),
				},
				ExpectedStorageClassInStatus: "sc",
			},
		),
		Entry(
			"Should be \"true\" status condition because sc in spec",
			handlerTestArgs{
				VirtualDisk:                  newVD(ptr.To("sc"), ""),
				StorageClassExistedInCluster: ptr.To("sc"),
				StorageClassInExistedPVC:     nil,
				ExpectedCondition: metav1.Condition{
					Status: metav1.ConditionTrue,
					Reason: vdcondition.StorageClassReady.String(),
				},
				ExpectedStorageClassInStatus: "sc",
			},
		),
		Entry(
			"Should be \"true\" status condition because has default sc",
			handlerTestArgs{
				VirtualDisk:                  newVD(nil, ""),
				StorageClassExistedInCluster: ptr.To("sc"),
				StorageClassInExistedPVC:     nil,
				ExpectedCondition: metav1.Condition{
					Status: metav1.ConditionTrue,
					Reason: vdcondition.StorageClassReady.String(),
				},
				ExpectedStorageClassInStatus: "sc",
			},
		),
		Entry(
			"Should be \"false\" status condition because sc from status not found",
			handlerTestArgs{
				VirtualDisk:                  newVD(nil, "statusSC"),
				StorageClassExistedInCluster: nil,
				StorageClassInExistedPVC:     nil,
				ExpectedCondition: metav1.Condition{
					Status: metav1.ConditionFalse,
					Reason: vdcondition.StorageClassNotFound.String(),
				},
				ExpectedStorageClassInStatus: "statusSC",
			},
		),
		Entry(
			"Should be pvc sc in status",
			handlerTestArgs{
				VirtualDisk:                  newVD(ptr.To("specSC"), "statusSC"),
				StorageClassExistedInCluster: ptr.To("pvcSC"),
				StorageClassInExistedPVC:     ptr.To("pvcSC"),
				ExpectedCondition: metav1.Condition{
					Status: metav1.ConditionTrue,
					Reason: vdcondition.StorageClassReady.String(),
				},
				ExpectedStorageClassInStatus: "pvcSC",
			},
		),
		Entry(
			"Should be pvc sc in status",
			handlerTestArgs{
				VirtualDisk:                  newVD(ptr.To("specSC"), "statusSC"),
				StorageClassExistedInCluster: ptr.To("pvcSC"),
				StorageClassInExistedPVC:     ptr.To("pvcSC"),
				ExpectedCondition: metav1.Condition{
					Status: metav1.ConditionTrue,
					Reason: vdcondition.StorageClassReady.String(),
				},
				ExpectedStorageClassInStatus: "pvcSC",
			},
		),
		Entry(
			"Should be spec sc in status",
			handlerTestArgs{
				VirtualDisk:                  newVD(ptr.To("specSC"), "statusSC"),
				StorageClassExistedInCluster: ptr.To("specSC"),
				StorageClassInExistedPVC:     nil,
				ExpectedCondition: metav1.Condition{
					Status: metav1.ConditionTrue,
					Reason: vdcondition.StorageClassReady.String(),
				},
				ExpectedStorageClassInStatus: "specSC",
			},
		),
	)
})

type handlerTestArgs struct {
	VirtualDisk                  *virtv2.VirtualDisk
	StorageClassInExistedPVC     *string
	StorageClassExistedInCluster *string
	ExpectedCondition            metav1.Condition
	ExpectedStorageClassInStatus string
}

func newDiskServiceMock(existedStorageClass, pvcSC *string) *DiskServiceMock {
	var diskServiceMock DiskServiceMock

	diskServiceMock.GetPersistentVolumeClaimFunc = func(_ context.Context, _ *supplements.Generator) (*corev1.PersistentVolumeClaim, error) {
		return newPVC(pvcSC), nil
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

func newVD(specSC *string, statusSC string) *virtv2.VirtualDisk {
	return &virtv2.VirtualDisk{
		Spec: virtv2.VirtualDiskSpec{
			PersistentVolumeClaim: virtv2.VirtualDiskPersistentVolumeClaim{
				StorageClass: specSC,
			},
		},
		Status: virtv2.VirtualDiskStatus{
			StorageClassName: statusSC,
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
