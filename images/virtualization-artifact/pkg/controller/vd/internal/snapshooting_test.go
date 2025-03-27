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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

var _ = DescribeTable("Test Handle cases", func(args SnapshottingHandlerTestHandlerArgs) {
	scheme := apiruntime.NewScheme()
	for _, f := range []func(*apiruntime.Scheme) error{
		virtv2.AddToScheme,
		virtv1.AddToScheme,
		corev1.AddToScheme,
	} {
		err := f(scheme)
		Expect(err).NotTo(HaveOccurred(), "failed to add scheme: %s", err)
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&args.Snapshot).Build()
	diskService := service.NewDiskService(fakeClient, nil, nil, "test")
	snapshottingHandler := NewSnapshottingHandler(diskService)

	vd := virtv2.VirtualDisk{
		ObjectMeta: metav1.ObjectMeta{
			DeletionTimestamp: args.DeletionTimestamp,
			Name:              "test-vd",
			Namespace:         "test-namespace",
		},
		Status: virtv2.VirtualDiskStatus{
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
	Entry("normal case", SnapshottingHandlerTestHandlerArgs{
		DeletionTimestamp: nil,
		ReadyCondition: metav1.Condition{
			Type:   vdcondition.ReadyType.String(),
			Status: metav1.ConditionTrue,
		},
		IsExpectCondition: false,
	}),
	Entry("not ready case", SnapshottingHandlerTestHandlerArgs{
		DeletionTimestamp: nil,
		ReadyCondition: metav1.Condition{
			Type:   vdcondition.ReadyType.String(),
			Status: metav1.ConditionFalse,
		},
		IsExpectCondition: false,
	}),
	Entry("unknown ready case", SnapshottingHandlerTestHandlerArgs{
		DeletionTimestamp: nil,
		ReadyCondition: metav1.Condition{
			Type:   vdcondition.ReadyType.String(),
			Status: metav1.ConditionUnknown,
		},
		IsExpectCondition:     false,
		ExpectConditionStatus: metav1.ConditionUnknown,
	}),
	Entry("not vd snapshots", SnapshottingHandlerTestHandlerArgs{
		DeletionTimestamp: nil,
		ReadyCondition: metav1.Condition{
			Type:   vdcondition.ReadyType.String(),
			Status: metav1.ConditionTrue,
		},
		Snapshot: virtv2.VirtualDiskSnapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-snapshot",
				Namespace: "test-namespace",
			},
			Spec: virtv2.VirtualDiskSnapshotSpec{
				VirtualDiskName: "test-vdd",
			},
		},
		IsExpectCondition: false,
	}),
	Entry("vd has snapshot but resized", SnapshottingHandlerTestHandlerArgs{
		DeletionTimestamp: nil,
		ReadyCondition: metav1.Condition{
			Type:   vdcondition.ReadyType.String(),
			Status: metav1.ConditionTrue,
		},
		ResizingCondition: metav1.Condition{
			Type:   vdcondition.ResizingType.String(),
			Status: metav1.ConditionTrue,
		},
		Snapshot: virtv2.VirtualDiskSnapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-snapshot",
				Namespace: "test-namespace",
			},
			Spec: virtv2.VirtualDiskSnapshotSpec{
				VirtualDiskName: "test-vd",
			},
		},
		IsExpectCondition:     true,
		ExpectConditionStatus: metav1.ConditionFalse,
	}),
	Entry("vd has snapshot", SnapshottingHandlerTestHandlerArgs{
		DeletionTimestamp: nil,
		ReadyCondition: metav1.Condition{
			Type:   vdcondition.ReadyType.String(),
			Status: metav1.ConditionTrue,
		},
		Snapshot: virtv2.VirtualDiskSnapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-snapshot",
				Namespace: "test-namespace",
			},
			Spec: virtv2.VirtualDiskSnapshotSpec{
				VirtualDiskName: "test-vd",
			},
		},
		IsExpectCondition:     true,
		ExpectConditionStatus: metav1.ConditionTrue,
	}),
	Entry("deletion case", SnapshottingHandlerTestHandlerArgs{
		DeletionTimestamp: &metav1.Time{
			Time: metav1.Now().Time,
		},
		IsExpectCondition: false,
	}),
)

type SnapshottingHandlerTestHandlerArgs struct {
	DeletionTimestamp     *metav1.Time
	ReadyCondition        metav1.Condition
	ResizingCondition     metav1.Condition
	Snapshot              virtv2.VirtualDiskSnapshot
	IsExpectCondition     bool
	ExpectConditionStatus metav1.ConditionStatus
}
