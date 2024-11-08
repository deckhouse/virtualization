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
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/source"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

var _ = Describe("LifeCycleHandler Run", func() {
	DescribeTable("Check lifeCycleCleanup calling",
		func(
			readyConditionStatus metav1.ConditionStatus,
			readyConditionReason string,
			scConditionReadyStatus metav1.ConditionStatus,
			changedMock bool,
			scInStatus string,
			needCleanUp bool,
		) {
			var sourcesMock SourcesMock
			cleanUpCalled := false
			vd := virtv2.VirtualDisk{
				Status: virtv2.VirtualDiskStatus{
					StorageClassName: scInStatus,
					Conditions: []metav1.Condition{
						{
							Type:   vdcondition.StorageClassReadyType,
							Status: scConditionReadyStatus,
						},
						{
							Type:   vdcondition.ReadyType,
							Status: readyConditionStatus,
							Reason: readyConditionReason,
						},
						{
							Type:   vdcondition.DatasourceReadyType,
							Status: metav1.ConditionTrue,
						},
					},
				},
			}

			sourcesMock.CleanUpFunc = func(ctx context.Context, vd *virtv2.VirtualDisk) (bool, error) {
				cleanUpCalled = true
				return false, nil
			}

			sourcesMock.ChangedFunc = func(ctx context.Context, vd *virtv2.VirtualDisk) bool {
				return changedMock
			}

			sourcesMock.GetFunc = func(dsType virtv2.DataSourceType) (source.Handler, bool) {
				return nil, false
			}

			handler := NewLifeCycleHandler(nil, &sourcesMock, nil)

			opts := slog.HandlerOptions{
				AddSource: true,
				Level:     slog.Level(-1),
			}
			slogHandler := slog.NewJSONHandler(os.Stderr, &opts)
			ctx := logger.ToContext(context.TODO(), slog.New(slogHandler))

			_, _ = handler.Handle(ctx, &vd)

			Expect(cleanUpCalled).To(Equal(needCleanUp))
		},
		Entry(
			"Should call cleanUp because changed spec",
			metav1.ConditionUnknown,
			conditions.ReasonUnknown.String(),
			metav1.ConditionUnknown,
			true,
			"",
			true,
		),
		Entry(
			"Should not call cleanUp because ready condition true",
			metav1.ConditionTrue,
			conditions.ReasonUnknown.String(),
			metav1.ConditionUnknown,
			true,
			"",
			false,
		),
		Entry(
			"Should not call cleanUp because ready condition reason Lost",
			metav1.ConditionUnknown,
			vdcondition.Lost,
			metav1.ConditionUnknown,
			true,
			"",
			false,
		),
		Entry(
			"Should not call cleanUp because spec not changed",
			metav1.ConditionUnknown,
			conditions.ReasonUnknown.String(),
			metav1.ConditionUnknown,
			false,
			"",
			false,
		),
		Entry(
			"Should call cleanUp because StorageClassReady condition turned to false",
			metav1.ConditionUnknown,
			conditions.ReasonUnknown.String(),
			metav1.ConditionUnknown,
			false,
			"sc",
			true,
		),
		Entry(
			"Should not call cleanUp because no StorageClass in status",
			metav1.ConditionUnknown,
			conditions.ReasonUnknown.String(),
			metav1.ConditionUnknown,
			false,
			"",
			false,
		),
		Entry(
			"Should not call cleanUp because ready condition true",
			metav1.ConditionTrue,
			conditions.ReasonUnknown.String(),
			metav1.ConditionUnknown,
			false,
			"sc",
			false,
		),
	)
})
