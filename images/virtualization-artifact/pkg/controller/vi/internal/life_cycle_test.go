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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/vi/internal/source"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

var _ = Describe("LifeCycleHandler Run", func() {
	DescribeTable(
		"Check on LifeCycle.Cleanup after spec changes",
		func(args cleanupAfterSpecChangeTestArgs) {
			args.ReadyCondition.Type = vicondition.ReadyType.String()
			var sourcesMock SourcesMock
			cleanUpCalled := false
			vi := v1alpha2.VirtualImage{
				Status: v1alpha2.VirtualImageStatus{
					Conditions: []metav1.Condition{
						args.ReadyCondition,
						{
							Type:   vicondition.StorageClassReadyType.String(),
							Status: metav1.ConditionTrue,
						},
						{
							Type:   vicondition.DatasourceReadyType.String(),
							Status: metav1.ConditionTrue,
						},
					},
				},
			}

			sourcesMock.CleanUpFunc = func(ctx context.Context, vd *v1alpha2.VirtualImage) (bool, error) {
				cleanUpCalled = true
				return false, nil
			}

			sourcesMock.ChangedFunc = func(contextMoqParam context.Context, vi *v1alpha2.VirtualImage) bool {
				return args.SpecChanged
			}

			sourcesMock.ForFunc = func(_ v1alpha2.DataSourceType) (source.Handler, bool) {
				var handler source.HandlerMock

				handler.StoreToPVCFunc = func(_ context.Context, _ *v1alpha2.VirtualImage) (reconcile.Result, error) {
					return reconcile.Result{}, nil
				}

				return &handler, false
			}

			recorder := &eventrecord.EventRecorderLoggerMock{
				EventFunc: func(_ client.Object, _, _, _ string) {},
			}

			handler := NewLifeCycleHandler(recorder, &sourcesMock, nil)

			_, _ = handler.Handle(context.TODO(), &vi)

			Expect(cleanUpCalled).To(Equal(args.ExpectCleanup))
		},
		Entry(
			"CleanUp should be called",
			cleanupAfterSpecChangeTestArgs{
				ReadyCondition: metav1.Condition{
					Status: metav1.ConditionUnknown,
				},
				SpecChanged:   true,
				ExpectCleanup: true,
			},
		),
		Entry(
			"CleanUp should not be called because ReadyCondition status is true",
			cleanupAfterSpecChangeTestArgs{
				ReadyCondition: metav1.Condition{
					Status: metav1.ConditionTrue,
				},
				SpecChanged:   true,
				ExpectCleanup: false,
			},
		),
		Entry(
			"CleanUp should not be called because spec is not changed",
			cleanupAfterSpecChangeTestArgs{
				ReadyCondition: metav1.Condition{
					Status: metav1.ConditionFalse,
				},
				SpecChanged:   false,
				ExpectCleanup: false,
			},
		),
	)

	DescribeTable(
		"Verification that LifeCycle.CleanUp is called after the StorageClassReady status becomes false",
		func(args cleanupAfterScNotReadyTestArgs) {
			args.ReadyCondition.Type = vicondition.ReadyType.String()
			args.StorageClassReadyCondition.Type = vicondition.StorageClassReadyType.String()
			var sourcesMock SourcesMock
			cleanUpCalled := false
			vi := v1alpha2.VirtualImage{
				Spec: v1alpha2.VirtualImageSpec{
					Storage: args.StorageType,
				},
				Status: v1alpha2.VirtualImageStatus{
					Conditions: []metav1.Condition{
						args.ReadyCondition,
						args.StorageClassReadyCondition,
						{
							Type:   vicondition.DatasourceReadyType.String(),
							Status: metav1.ConditionTrue,
						},
					},
					StorageClassName: args.StorageClassInStatus,
				},
			}

			sourcesMock.CleanUpFunc = func(ctx context.Context, vd *v1alpha2.VirtualImage) (bool, error) {
				cleanUpCalled = true
				return false, nil
			}

			sourcesMock.ChangedFunc = func(contextMoqParam context.Context, vi *v1alpha2.VirtualImage) bool {
				return false
			}

			sourcesMock.ForFunc = func(_ v1alpha2.DataSourceType) (source.Handler, bool) {
				var handler source.HandlerMock

				handler.StoreToPVCFunc = func(_ context.Context, _ *v1alpha2.VirtualImage) (reconcile.Result, error) {
					return reconcile.Result{}, nil
				}

				return &handler, false
			}

			handler := NewLifeCycleHandler(nil, &sourcesMock, nil)

			_, _ = handler.Handle(context.TODO(), &vi)

			Expect(cleanUpCalled).To(Equal(args.ExpectCleanup))
		},
		Entry(
			"CleanUp should not be called because DVCR storage type used",
			cleanupAfterScNotReadyTestArgs{
				ReadyCondition: metav1.Condition{
					Status: metav1.ConditionFalse,
				},
				StorageClassReadyCondition: metav1.Condition{
					Status: metav1.ConditionFalse,
				},
				StorageClassInStatus: "sc",
				StorageType:          v1alpha2.StorageContainerRegistry,
				ExpectCleanup:        false,
			},
		),
		Entry(
			"CleanUp should not be called because there is no sc in status",
			cleanupAfterScNotReadyTestArgs{
				ReadyCondition: metav1.Condition{
					Status: metav1.ConditionFalse,
				},
				StorageClassReadyCondition: metav1.Condition{
					Status: metav1.ConditionFalse,
				},
				StorageClassInStatus: "",
				StorageType:          v1alpha2.StoragePersistentVolumeClaim,
				ExpectCleanup:        false,
			},
		),
		Entry(
			"CleanUp should not be called because ReadyCondition status is true",
			cleanupAfterScNotReadyTestArgs{
				ReadyCondition: metav1.Condition{
					Status: metav1.ConditionTrue,
				},
				StorageClassReadyCondition: metav1.Condition{
					Status: metav1.ConditionFalse,
				},
				StorageClassInStatus: "sc",
				StorageType:          v1alpha2.StoragePersistentVolumeClaim,
				ExpectCleanup:        false,
			},
		),
		Entry(
			"Should not to call cleanup because StorageClassReady condition status is true",
			cleanupAfterScNotReadyTestArgs{
				ReadyCondition: metav1.Condition{
					Status: metav1.ConditionFalse,
				},
				StorageClassReadyCondition: metav1.Condition{
					Status: metav1.ConditionTrue,
				},
				StorageClassInStatus: "sc",
				StorageType:          v1alpha2.StoragePersistentVolumeClaim,
				ExpectCleanup:        false,
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
	StorageClassInStatus       string
	StorageType                v1alpha2.StorageType
	ExpectCleanup              bool
}
