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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/source"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

var _ = Describe("LifeCycleHandler Run", func() {
	DescribeTable(
		"Check on LifeCycle.Cleanup after spec changes",
		func(args cleanupAfterSpecChangeTestArgs) {
			var sourcesMock SourcesMock
			args.ReadyCondition.Type = vdcondition.ReadyType.String()
			cleanUpCalled := false
			vd := virtv2.VirtualDisk{
				Status: virtv2.VirtualDiskStatus{
					StorageClassName: "",
					Conditions: []metav1.Condition{
						args.ReadyCondition,
						{
							Type:   vdcondition.DatasourceReadyType.String(),
							Status: metav1.ConditionTrue,
						},
						{
							Type:   vdcondition.StorageClassReadyType.String(),
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
				var handler HandlerMock

				handler.SyncFunc = func(_ context.Context, _ *virtv2.VirtualDisk) (reconcile.Result, error) {
					return reconcile.Result{}, nil
				}

				return &handler, false
			}

			recorder := &eventrecord.EventRecorderLoggerMock{
				EventFunc: func(_ client.Object, _, _, _ string) {},
			}
			handler := NewLifeCycleHandler(recorder, nil, &sourcesMock, nil, nil)

			ctx := logger.ToContext(context.TODO(), slog.Default())

			_, _ = handler.Handle(ctx, &vd)

			Expect(cleanUpCalled).Should(Equal(args.ExpectCleanup))
		},
		Entry(
			"CleanUp should be called",
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
			"CleanUp should not be called because the spec has not changed",
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
			"CleanUp should not be called because ReadyCondition status is true",
			cleanupAfterSpecChangeTestArgs{
				ReadyCondition: metav1.Condition{
					Status: metav1.ConditionTrue,
					Reason: vdcondition.Ready.String(),
				},
				SpecChanged:   true,
				ExpectCleanup: false,
			},
		),
		Entry(
			"CleanUp should not be called because ReadyCondition reason is Lost",
			cleanupAfterSpecChangeTestArgs{
				ReadyCondition: metav1.Condition{
					Status: metav1.ConditionUnknown,
					Reason: vdcondition.Lost.String(),
				},
				SpecChanged:   true,
				ExpectCleanup: false,
			},
		),
	)

	DescribeTable(
		"Verification that LifeCycle.CleanUp is called after the StorageClassReady status becomes false",
		func(args cleanupAfterScNotReadyTestArgs) {
			args.ReadyCondition.Type = vdcondition.ReadyType.String()
			args.StorageClassReadyCondition.Type = vdcondition.StorageClassReadyType.String()
			var sourcesMock SourcesMock
			cleanUpCalled := false
			vd := virtv2.VirtualDisk{
				Status: virtv2.VirtualDiskStatus{
					Conditions: []metav1.Condition{
						args.ReadyCondition,
						args.StorageClassReadyCondition,
						{
							Type:   vdcondition.DatasourceReadyType.String(),
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
				var handler HandlerMock

				handler.SyncFunc = func(_ context.Context, _ *virtv2.VirtualDisk) (reconcile.Result, error) {
					return reconcile.Result{}, nil
				}

				return &handler, false
			}

			recorder := &eventrecord.EventRecorderLoggerMock{
				EventFunc: func(_ client.Object, _, _, _ string) {},
			}

			handler := NewLifeCycleHandler(recorder, nil, &sourcesMock, nil, nil)

			ctx := logger.ToContext(context.TODO(), slog.Default())

			_, _ = handler.Handle(ctx, &vd)

			Expect(cleanUpCalled).To(Equal(args.ExpectCleanup))
		},
		Entry(
			"CleanUp should not be called because StorageClassReady status is true",
			cleanupAfterScNotReadyTestArgs{
				ReadyCondition: metav1.Condition{
					Status: metav1.ConditionFalse,
				},
				StorageClassReadyCondition: metav1.Condition{
					Status: metav1.ConditionTrue,
				},
				ExpectCleanup: false,
			},
		),
		Entry(
			"CleanUp should not be called because Ready status is true",
			cleanupAfterScNotReadyTestArgs{
				ReadyCondition: metav1.Condition{
					Status: metav1.ConditionTrue,
				},
				StorageClassReadyCondition: metav1.Condition{
					Status: metav1.ConditionFalse,
				},
				ExpectCleanup: false,
			},
		),
		Entry(
			"CleanUp should not be called because StorageClass in status is empty",
			cleanupAfterScNotReadyTestArgs{
				ReadyCondition: metav1.Condition{
					Status: metav1.ConditionFalse,
				},
				StorageClassReadyCondition: metav1.Condition{
					Status: metav1.ConditionFalse,
				},
				ExpectCleanup: false,
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
	ExpectCleanup              bool
}
