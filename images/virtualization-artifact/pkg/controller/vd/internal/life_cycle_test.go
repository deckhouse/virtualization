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
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/source"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

var _ = Describe("LifeCycleHandler Run", func() {
	DescribeTable(
		"Check on LifeCycle.Cleanup after spec changes",
		func(args cleanupAfterSpecChangeTestArgs) {
			var sourcesMock SourcesMock
			args.ReadyCondition.Type = vdcondition.ReadyType.String()
			cleanUpCalled := false
			vd := v1alpha2.VirtualDisk{
				Status: v1alpha2.VirtualDiskStatus{
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
				Spec: v1alpha2.VirtualDiskSpec{
					DataSource: &v1alpha2.VirtualDiskDataSource{
						Type: v1alpha2.DataSourceTypeHTTP,
					},
				},
			}

			sourcesMock.CleanUpFunc = func(ctx context.Context, vd *v1alpha2.VirtualDisk) (bool, error) {
				cleanUpCalled = true
				return false, nil
			}

			sourcesMock.ChangedFunc = func(ctx context.Context, vd *v1alpha2.VirtualDisk) bool {
				return args.SpecChanged
			}

			sourcesMock.GetFunc = func(dsType v1alpha2.DataSourceType) (source.Handler, bool) {
				var handler HandlerMock

				handler.SyncFunc = func(_ context.Context, _ *v1alpha2.VirtualDisk) (reconcile.Result, error) {
					return reconcile.Result{}, nil
				}

				return &handler, false
			}

			recorder := &eventrecord.EventRecorderLoggerMock{
				EventFunc: func(_ client.Object, _, _, _ string) {},
			}
			handler := NewLifeCycleHandler(recorder, nil, &sourcesMock, nil)

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
			vd := v1alpha2.VirtualDisk{
				Status: v1alpha2.VirtualDiskStatus{
					Conditions: []metav1.Condition{
						args.ReadyCondition,
						args.StorageClassReadyCondition,
						{
							Type:   vdcondition.DatasourceReadyType.String(),
							Status: metav1.ConditionTrue,
						},
					},
				},
				Spec: v1alpha2.VirtualDiskSpec{
					DataSource: &v1alpha2.VirtualDiskDataSource{
						Type: v1alpha2.DataSourceTypeHTTP,
					},
				},
			}

			sourcesMock.CleanUpFunc = func(ctx context.Context, vd *v1alpha2.VirtualDisk) (bool, error) {
				cleanUpCalled = true
				return false, nil
			}

			sourcesMock.ChangedFunc = func(ctx context.Context, vd *v1alpha2.VirtualDisk) bool {
				return false
			}

			sourcesMock.GetFunc = func(dsType v1alpha2.DataSourceType) (source.Handler, bool) {
				var handler HandlerMock

				handler.SyncFunc = func(_ context.Context, _ *v1alpha2.VirtualDisk) (reconcile.Result, error) {
					return reconcile.Result{}, nil
				}

				return &handler, false
			}

			recorder := &eventrecord.EventRecorderLoggerMock{
				EventFunc: func(_ client.Object, _, _, _ string) {},
			}

			handler := NewLifeCycleHandler(recorder, nil, &sourcesMock, nil)

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

	DescribeTable(
		"Message propagation and DatasourceIsNotFound reason",
		func(datasourceReason, expectedReadyReason string) {
			var sourcesMock SourcesMock
			recorder := &eventrecord.EventRecorderLoggerMock{
				EventFunc: func(_ client.Object, _, _, _ string) {},
			}
			ctx := logger.ToContext(context.TODO(), testutil.NewNoOpSlogLogger())
			vd := v1alpha2.VirtualDisk{
				Status: v1alpha2.VirtualDiskStatus{
					Conditions: []metav1.Condition{
						{
							Type:    vdcondition.DatasourceReadyType.String(),
							Status:  metav1.ConditionFalse,
							Reason:  datasourceReason,
							Message: "Test message",
						},
						{
							Type:   vdcondition.StorageClassReadyType.String(),
							Status: metav1.ConditionTrue,
						},
					},
				},
			}

			sourcesMock.ChangedFunc = func(_ context.Context, _ *v1alpha2.VirtualDisk) bool {
				return false
			}
			sourcesMock.GetFunc = func(_ v1alpha2.DataSourceType) (source.Handler, bool) {
				return &source.HandlerMock{SyncFunc: func(_ context.Context, _ *v1alpha2.VirtualDisk) (reconcile.Result, error) {
					return reconcile.Result{}, nil
				}}, true
			}
			handler := NewLifeCycleHandler(recorder, nil, &sourcesMock, nil)
			_, _ = handler.Handle(ctx, &vd)
			readyCond, _ := conditions.GetCondition(vdcondition.ReadyType, vd.Status.Conditions)
			Expect(readyCond.Reason).Should(Equal(expectedReadyReason))
			Expect(readyCond.Message).Should(Equal("Test message"))
		},
		Entry(
			"Generic not ready, propagate message",
			vdcondition.ImageNotReady.String(),
			vdcondition.DatasourceIsNotReady.String(),
		),
		Entry(
			"Image not found, use DatasourceIsNotFound and propagate",
			vdcondition.ImageNotFound.String(),
			vdcondition.DatasourceIsNotFound.String(),
		),
		Entry(
			"Cluster image not found, use DatasourceIsNotFound and propagate",
			vdcondition.ClusterImageNotFound.String(),
			vdcondition.DatasourceIsNotFound.String(),
		),
	)

	It("should handle a VirtualDisk without data source", func() {
		var sourcesMock SourcesMock
		recorder := &eventrecord.EventRecorderLoggerMock{
			EventFunc: func(_ client.Object, _, _, _ string) {},
		}
		ctx := logger.ToContext(context.TODO(), testutil.NewNoOpSlogLogger())
		syncCalled := false
		blank := &source.HandlerMock{
			SyncFunc: func(_ context.Context, _ *v1alpha2.VirtualDisk) (reconcile.Result, error) {
				syncCalled = true
				return reconcile.Result{}, nil
			},
		}
		vd := v1alpha2.VirtualDisk{
			Status: v1alpha2.VirtualDiskStatus{
				StorageClassName: "vd-sc",
				Conditions: []metav1.Condition{
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
		}

		sourcesMock.ChangedFunc = func(_ context.Context, _ *v1alpha2.VirtualDisk) bool {
			return false
		}
		handler := NewLifeCycleHandler(recorder, blank, &sourcesMock, nil)

		Expect(func() {
			_, _ = handler.Handle(ctx, &vd)
		}).NotTo(Panic())
		Expect(syncCalled).To(BeTrue())
	})

	It("should set a dedicated reason when storage class does not match the source virtual image", func() {
		scheme := runtime.NewScheme()
		Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
		Expect(storagev1.AddToScheme(scheme)).To(Succeed())

		vi := &v1alpha2.VirtualImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "source-vi",
				Namespace: "default",
			},
			Spec: v1alpha2.VirtualImageSpec{
				Storage: v1alpha2.StoragePersistentVolumeClaim,
			},
			Status: v1alpha2.VirtualImageStatus{
				Phase:            v1alpha2.ImageReady,
				StorageClassName: "vi-sc",
			},
		}

		vdSC := &storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vd-sc",
			},
			Provisioner: "first.csi.example.com",
		}

		viSC := &storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vi-sc",
			},
			Provisioner: "second.csi.example.com",
		}

		k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vi, vdSC, viSC).Build()
		var sourcesMock SourcesMock
		sourcesMock.ChangedFunc = func(_ context.Context, _ *v1alpha2.VirtualDisk) bool {
			return false
		}
		recorder := &eventrecord.EventRecorderLoggerMock{
			EventFunc: func(_ client.Object, _, _, _ string) {},
		}
		ctx := logger.ToContext(context.TODO(), testutil.NewNoOpSlogLogger())
		vd := v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
			},
			Spec: v1alpha2.VirtualDiskSpec{
				DataSource: &v1alpha2.VirtualDiskDataSource{
					Type: v1alpha2.DataSourceTypeObjectRef,
					ObjectRef: &v1alpha2.VirtualDiskObjectRef{
						Kind: v1alpha2.VirtualDiskObjectRefKindVirtualImage,
						Name: vi.Name,
					},
				},
			},
			Status: v1alpha2.VirtualDiskStatus{
				StorageClassName: "vd-sc",
				Conditions: []metav1.Condition{
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
		}

		handler := NewLifeCycleHandler(recorder, &source.HandlerMock{}, &sourcesMock, k8sClient)
		_, err := handler.Handle(ctx, &vd)
		Expect(err).NotTo(HaveOccurred())

		readyCond, ok := conditions.GetCondition(vdcondition.ReadyType, vd.Status.Conditions)
		Expect(ok).To(BeTrue())
		Expect(readyCond.Reason).To(Equal(vdcondition.StorageClassProvisionerMismatch.String()))
		Expect(readyCond.Message).To(Equal(`Virtual disk storage class "vd-sc" provisioner does not match virtual image storage class "vi-sc" provisioner: source type with different provisioners is not supported yet`))
	})
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
