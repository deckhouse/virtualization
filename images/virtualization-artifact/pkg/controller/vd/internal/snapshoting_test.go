/*
Copyright 2025 Flant JSC

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

var _ = DescribeTable("Test Handle cases", func(args snapshottingHandlerTestHandlerArgs) {
	fakeClient, err := testutil.NewFakeClientWithObjects(&args.Snapshot)
	Expect(err).NotTo(HaveOccurred(), "failed to create fake client: %s", err)
	diskService := service.NewDiskService(fakeClient, nil, nil, "test")
	snapshottingHandler := NewSnapshottingHandler(diskService)

	vd := v1alpha2.VirtualDisk{
		ObjectMeta: metav1.ObjectMeta{
			DeletionTimestamp: args.DeletionTimestamp,
			Name:              "test-vd",
			Namespace:         "test-namespace",
		},
		Status: v1alpha2.VirtualDiskStatus{
			Conditions: []metav1.Condition{
				args.ReadyCondition,
				args.ResizingCondition,
			},
		},
	}

	result, err := snapshottingHandler.Handle(context.Background(), &vd)
	Expect(err).NotTo(HaveOccurred())
	Expect(result).To(Equal(reconcile.Result{}))
	snapshottingCondition, ok := conditions.GetCondition(vdcondition.SnapshottingType, vd.Status.Conditions)
	Expect(ok).Should(Equal(args.IsExpectCondition))
	if args.IsExpectCondition {
		Expect(snapshottingCondition.Status).Should(Equal(args.ExpectConditionStatus))
	}
},
	Entry("normal case", snapshottingHandlerTestHandlerArgs{
		DeletionTimestamp: nil,
		ReadyCondition: metav1.Condition{
			Type:   vdcondition.ReadyType.String(),
			Status: metav1.ConditionTrue,
		},
		IsExpectCondition: false,
	}),
	Entry("not ready case", snapshottingHandlerTestHandlerArgs{
		DeletionTimestamp: nil,
		ReadyCondition: metav1.Condition{
			Type:   vdcondition.ReadyType.String(),
			Status: metav1.ConditionFalse,
		},
		IsExpectCondition: false,
	}),
	Entry("unknown ready case", snapshottingHandlerTestHandlerArgs{
		DeletionTimestamp: nil,
		ReadyCondition: metav1.Condition{
			Type:   vdcondition.ReadyType.String(),
			Status: metav1.ConditionUnknown,
		},
		IsExpectCondition:     false,
		ExpectConditionStatus: metav1.ConditionUnknown,
	}),
	Entry("not vd snapshots", snapshottingHandlerTestHandlerArgs{
		DeletionTimestamp: nil,
		ReadyCondition: metav1.Condition{
			Type:   vdcondition.ReadyType.String(),
			Status: metav1.ConditionTrue,
		},
		Snapshot: v1alpha2.VirtualDiskSnapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-snapshot",
				Namespace: "test-namespace",
			},
			Spec: v1alpha2.VirtualDiskSnapshotSpec{
				VirtualDiskName: "test-vdd",
			},
		},
		IsExpectCondition: false,
	}),
	Entry("vd has snapshot but resized", snapshottingHandlerTestHandlerArgs{
		DeletionTimestamp: nil,
		ReadyCondition: metav1.Condition{
			Type:   vdcondition.ReadyType.String(),
			Status: metav1.ConditionTrue,
		},
		ResizingCondition: metav1.Condition{
			Type:   vdcondition.ResizingType.String(),
			Status: metav1.ConditionTrue,
		},
		Snapshot: v1alpha2.VirtualDiskSnapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-snapshot",
				Namespace: "test-namespace",
			},
			Spec: v1alpha2.VirtualDiskSnapshotSpec{
				VirtualDiskName: "test-vd",
			},
		},
		IsExpectCondition:     true,
		ExpectConditionStatus: metav1.ConditionFalse,
	}),
	Entry("vd has snapshot", snapshottingHandlerTestHandlerArgs{
		DeletionTimestamp: nil,
		ReadyCondition: metav1.Condition{
			Type:   vdcondition.ReadyType.String(),
			Status: metav1.ConditionTrue,
		},
		Snapshot: v1alpha2.VirtualDiskSnapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-snapshot",
				Namespace: "test-namespace",
			},
			Spec: v1alpha2.VirtualDiskSnapshotSpec{
				VirtualDiskName: "test-vd",
			},
		},
		IsExpectCondition:     true,
		ExpectConditionStatus: metav1.ConditionTrue,
	}),
	Entry("deletion case", snapshottingHandlerTestHandlerArgs{
		DeletionTimestamp: &metav1.Time{
			Time: metav1.Now().Time,
		},
		IsExpectCondition: false,
	}),
)

type snapshottingHandlerTestHandlerArgs struct {
	DeletionTimestamp     *metav1.Time
	ReadyCondition        metav1.Condition
	ResizingCondition     metav1.Condition
	Snapshot              v1alpha2.VirtualDiskSnapshot
	IsExpectCondition     bool
	ExpectConditionStatus metav1.ConditionStatus
}
