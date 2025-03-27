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
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/source"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

func TestDatasourceReadyHandler_Handle(t *testing.T) {
	ctx := context.TODO()
	blank := &HandlerMock{
		ValidateFunc: func(_ context.Context, _ *virtv2.VirtualDisk) error {
			return nil
		},
	}
	sources := &SourcesMock{
		GetFunc: func(dsType virtv2.DataSourceType) (source.Handler, bool) {
			return blank, true
		},
	}
	recorder := &eventrecord.EventRecorderLoggerMock{
		EventFunc: func(_ client.Object, _, _, _ string) {},
	}

	t.Run("VirtualDisk with DeletionTimestamp", func(t *testing.T) {
		vd := virtv2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{
				DeletionTimestamp: &metav1.Time{Time: metav1.Now().Time},
			},
		}

		handler := NewDatasourceReadyHandler(recorder, nil, nil)
		_, err := handler.Handle(ctx, &vd)
		require.NoError(t, err)
	})

	t.Run("VirtualDisk with Blank DataSource", func(t *testing.T) {
		vd := virtv2.VirtualDisk{}

		handler := NewDatasourceReadyHandler(recorder, blank, nil)
		_, err := handler.Handle(ctx, &vd)
		require.NoError(t, err)

		condition := vd.Status.Conditions[0]
		require.Equal(t, vdcondition.DatasourceReadyType.String(), condition.Type)
		require.Equal(t, metav1.ConditionTrue, condition.Status)
		require.Equal(t, vdcondition.DatasourceReady.String(), condition.Reason)
	})

	t.Run("VirtualDisk with Non Blank DataSource", func(t *testing.T) {
		vd := virtv2.VirtualDisk{
			Spec: virtv2.VirtualDiskSpec{
				DataSource: &virtv2.VirtualDiskDataSource{
					Type: "NonBlank",
				},
			},
		}

		handler := NewDatasourceReadyHandler(recorder, nil, sources)
		_, err := handler.Handle(ctx, &vd)
		require.NoError(t, err)

		condition := vd.Status.Conditions[0]
		require.Equal(t, vdcondition.DatasourceReadyType.String(), condition.Type)
		require.Equal(t, metav1.ConditionTrue, condition.Status)
		require.Equal(t, vdcondition.DatasourceReady.String(), condition.Reason)
	})
}
