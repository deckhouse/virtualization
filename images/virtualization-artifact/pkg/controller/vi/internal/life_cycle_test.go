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
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization-controller/pkg/controller/vi/internal/source"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("LifeCycleHandler Run", func() {
	DescribeTable("Check lifeCycle Cleanup calling",
		func(
			readyConditionStatus, scConditionReadyStatus metav1.ConditionStatus,
			changedMock bool,
			scInStatus string,
			storageType virtv2.StorageType,
			needCleanup bool,
		) {
			var sourcesMock SourcesMock
			cleanUpCalled := false
			vi := virtv2.VirtualImage{
				Spec: virtv2.VirtualImageSpec{
					Storage: storageType,
				},
				Status: virtv2.VirtualImageStatus{
					StorageClassName: scInStatus,
					Conditions: []metav1.Condition{
						{
							Type:   vicondition.StorageClassReadyType,
							Status: scConditionReadyStatus,
						},
						{
							Type:   vicondition.ReadyType,
							Status: readyConditionStatus,
						},
						{
							Type:   vicondition.DatasourceReadyType,
							Status: metav1.ConditionTrue,
						},
					},
				},
			}

			sourcesMock.CleanUpFunc = func(ctx context.Context, vd *virtv2.VirtualImage) (bool, error) {
				cleanUpCalled = true
				return false, nil
			}

			sourcesMock.ChangedFunc = func(contextMoqParam context.Context, vi *virtv2.VirtualImage) bool {
				return changedMock
			}

			sourcesMock.ForFunc = func(_ virtv2.DataSourceType) (source.Handler, bool) {
				return nil, false
			}

			handler := NewLifeCycleHandler(&sourcesMock, nil)

			_, _ = handler.Handle(context.TODO(), &vi)

			Expect(cleanUpCalled).To(Equal(needCleanup))
		},
		Entry(
			"Should call cleanup",
			metav1.ConditionUnknown,
			metav1.ConditionUnknown,
			true,
			"",
			virtv2.StorageContainerRegistry,
			true,
		),
		Entry(
			"Should not call cleanup because ready condition true",
			metav1.ConditionTrue,
			metav1.ConditionUnknown,
			true,
			"",
			virtv2.StorageContainerRegistry,
			false,
		),
		Entry(
			"Should call cleanup",
			metav1.ConditionUnknown,
			metav1.ConditionUnknown,
			false,
			"hasClass",
			virtv2.StorageKubernetes,
			true,
		),
		Entry(
			"Should not call because ready condition true",
			metav1.ConditionUnknown,
			metav1.ConditionTrue,
			false,
			"hasClass",
			virtv2.StorageKubernetes,
			false,
		),
		Entry(
			"Should not call cleanup because storageClass ready condition true",
			metav1.ConditionTrue,
			metav1.ConditionUnknown,
			false,
			"hasClass",
			virtv2.StorageKubernetes,
			false,
		),
		Entry(
			"Should not call cleanup because not storage class in status",
			metav1.ConditionUnknown,
			metav1.ConditionUnknown,
			false,
			"",
			virtv2.StorageKubernetes,
			false,
		),
		Entry(
			"Should not call cleanup because dvcr storage type",
			metav1.ConditionUnknown,
			metav1.ConditionUnknown,
			false,
			"hasClass",
			virtv2.StorageContainerRegistry,
			false,
		),
	)
})
