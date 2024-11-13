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
	"log/slog"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/source"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

var _ = Describe("LifeCycleHandler Run", func() {
	DescribeTable(
		"Check LifeCycleCleanup calling after spec changes",
		func(args cleanupAfterSpecChangeTestArgs) {
			var sourcesMock SourcesMock
			args.ReadyCondition.Type = vdcondition.ReadyType
			cleanUpCalled := false
			vd := virtv2.VirtualDisk{
				Status: virtv2.VirtualDiskStatus{
					StorageClassName: "",
					Conditions: []metav1.Condition{
						args.ReadyCondition,
						{
							Type:   vdcondition.DatasourceReadyType,
							Status: metav1.ConditionTrue,
						},
						{
							Type:   vdcondition.StorageClassReadyType,
							Status: metav1.ConditionTrue,
						},
					},
				},
				Spec: virtv2.VirtualDiskSpec{
					DataSource: &virtv2.VirtualDiskDataSource{
						Type: virtv2.DataSourceTypeHTTP,
					},
				},
			}

			sourcesMock.CleanUpFunc = func(ctx context.Context, vd *virtv2.VirtualDisk) (bool, error) {
				cleanUpCalled = true
				return false, nil
			}

			sourcesMock.ChangedFunc = func(ctx context.Context, vd *virtv2.VirtualDisk) bool {
				return args.SpecChanged
			}

			sourcesMock.GetFunc = func(dsType virtv2.DataSourceType) (source.Handler, bool) {
				var handler source.HandlerMock

				handler.SyncFunc = func(_ context.Context, _ *virtv2.VirtualDisk) (reconcile.Result, error) {
					return reconcile.Result{}, nil
				}

				return &handler, false
			}

			handler := NewLifeCycleHandler(nil, &sourcesMock, nil)

			ctx := logger.ToContext(context.TODO(), slog.Default())

			_, _ = handler.Handle(ctx, &vd)

			Expect(cleanUpCalled).Should(Equal(args.ExpectCleanup))
		},
		Entry(
			"Should to call cleanup",
			cleanupAfterSpecChangeTestArgs{
				ReadyCondition: metav1.Condition{
					Status: metav1.ConditionUnknown,
					Reason: conditions.ReasonUnknown.String(),
				},
				SpecChanged:   true,
				ExpectCleanup: true,
			},
		),
		Entry(
			"Should not to call cleanup because spec has not changed",
			cleanupAfterSpecChangeTestArgs{
				ReadyCondition: metav1.Condition{
					Status: metav1.ConditionUnknown,
					Reason: conditions.ReasonUnknown.String(),
				},
				SpecChanged:   false,
				ExpectCleanup: false,
			},
		),
		Entry(
			"Should not to call cleanup because readyCondition status is true",
			cleanupAfterSpecChangeTestArgs{
				ReadyCondition: metav1.Condition{
					Status: metav1.ConditionTrue,
					Reason: vdcondition.Ready,
				},
				SpecChanged:   true,
				ExpectCleanup: false,
			},
		),
		Entry(
			"Should not to call cleanup because readyCondition reason is Lost",
			cleanupAfterSpecChangeTestArgs{
				ReadyCondition: metav1.Condition{
					Status: metav1.ConditionUnknown,
					Reason: vdcondition.Lost,
				},
				SpecChanged:   true,
				ExpectCleanup: false,
			},
		),
	)

	DescribeTable(
		"Check LifeCycleCleanup calling after StorageClassReady set to false",
		func(args cleanupAfterScNotReadyTestArgs) {
			args.ReadyCondition.Type = vdcondition.ReadyType
			args.StorageClassReadyCondition.Type = vdcondition.StorageClassReadyType
			var sourcesMock SourcesMock
			cleanUpCalled := false
			vd := virtv2.VirtualDisk{
				Status: virtv2.VirtualDiskStatus{
					StorageClassName: args.StorageClassInStatusName,
					Conditions: []metav1.Condition{
						args.ReadyCondition,
						args.StorageClassReadyCondition,
						{
							Type:   vdcondition.DatasourceReadyType,
							Status: metav1.ConditionTrue,
						},
					},
				},
				Spec: virtv2.VirtualDiskSpec{
					DataSource: &virtv2.VirtualDiskDataSource{
						Type: virtv2.DataSourceTypeHTTP,
					},
				},
			}

			sourcesMock.CleanUpFunc = func(ctx context.Context, vd *virtv2.VirtualDisk) (bool, error) {
				cleanUpCalled = true
				return false, nil
			}

			sourcesMock.ChangedFunc = func(ctx context.Context, vd *virtv2.VirtualDisk) bool {
				return false
			}

			sourcesMock.GetFunc = func(dsType virtv2.DataSourceType) (source.Handler, bool) {
				var handler source.HandlerMock

				handler.SyncFunc = func(_ context.Context, _ *virtv2.VirtualDisk) (reconcile.Result, error) {
					return reconcile.Result{}, nil
				}

				return &handler, false
			}

			handler := NewLifeCycleHandler(nil, &sourcesMock, nil)

			ctx := logger.ToContext(context.TODO(), slog.Default())

			_, _ = handler.Handle(ctx, &vd)

			Expect(cleanUpCalled).To(Equal(args.ExpectCleanup))
		},
		Entry(
			"Should to call cleanup because StorageClassReady status is false",
			cleanupAfterScNotReadyTestArgs{
				ReadyCondition: metav1.Condition{
					Status: metav1.ConditionFalse,
				},
				StorageClassReadyCondition: metav1.Condition{
					Status: metav1.ConditionFalse,
				},
				StorageClassInStatusName: "sc",
				ExpectCleanup:            true,
			},
		),
		Entry(
			"Should not to call cleanup because StorageClassReady status is true",
			cleanupAfterScNotReadyTestArgs{
				ReadyCondition: metav1.Condition{
					Status: metav1.ConditionFalse,
				},
				StorageClassReadyCondition: metav1.Condition{
					Status: metav1.ConditionTrue,
				},
				StorageClassInStatusName: "sc",
				ExpectCleanup:            false,
			},
		),
		Entry(
			"Should to call cleanup because Ready status is true",
			cleanupAfterScNotReadyTestArgs{
				ReadyCondition: metav1.Condition{
					Status: metav1.ConditionTrue,
				},
				StorageClassReadyCondition: metav1.Condition{
					Status: metav1.ConditionFalse,
				},
				StorageClassInStatusName: "sc",
				ExpectCleanup:            false,
			},
		),
		Entry(
			"Should to call cleanup because StorageClass in status is empty",
			cleanupAfterScNotReadyTestArgs{
				ReadyCondition: metav1.Condition{
					Status: metav1.ConditionFalse,
				},
				StorageClassReadyCondition: metav1.Condition{
					Status: metav1.ConditionFalse,
				},
				StorageClassInStatusName: "",
				ExpectCleanup:            false,
			},
		),
	)
})

type cleanupAfterSpecChangeTestArgs struct {
	ReadyCondition metav1.Condition
	SpecChanged    bool
	ExpectCleanup  bool
}

type cleanupAfterScNotReadyTestArgs struct {
	ReadyCondition             metav1.Condition
	StorageClassReadyCondition metav1.Condition
	StorageClassInStatusName   string
	ExpectCleanup              bool
}
