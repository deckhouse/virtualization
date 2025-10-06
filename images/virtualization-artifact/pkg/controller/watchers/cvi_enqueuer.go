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

package watchers

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/cvicondition"
)

type ClusterVirtualImageRequestEnqueuer struct {
	enqueueFromObj  client.Object
	enqueueFromKind v1alpha2.ClusterVirtualImageObjectRefKind
	client          client.Client
	logger          *log.Logger
}

func NewClusterVirtualImageRequestEnqueuer(client client.Client, enqueueFromObj client.Object, enqueueFromKind v1alpha2.ClusterVirtualImageObjectRefKind) *ClusterVirtualImageRequestEnqueuer {
	return &ClusterVirtualImageRequestEnqueuer{
		enqueueFromObj:  enqueueFromObj,
		enqueueFromKind: enqueueFromKind,
		client:          client,
		logger:          log.Default().With("enqueuer", "cvi"),
	}
}

func (w ClusterVirtualImageRequestEnqueuer) GetEnqueueFrom() client.Object {
	return w.enqueueFromObj
}

func (w ClusterVirtualImageRequestEnqueuer) EnqueueRequests(ctx context.Context, obj client.Object) (requests []reconcile.Request) {
	var cvis v1alpha2.ClusterVirtualImageList
	err := w.client.List(ctx, &cvis)
	if err != nil {
		w.logger.Error(fmt.Sprintf("failed to list cvi: %s", err))
		return
	}

	for _, cvi := range cvis.Items {
		dsReady, _ := conditions.GetCondition(cvicondition.DatasourceReadyType, cvi.Status.Conditions)
		if dsReady.Status == metav1.ConditionTrue {
			continue
		}

		if cvi.Spec.DataSource.Type != v1alpha2.DataSourceTypeObjectRef {
			continue
		}

		ref := cvi.Spec.DataSource.ObjectRef

		if ref == nil || ref.Kind != w.enqueueFromKind {
			continue
		}

		if ref.Name == obj.GetName() {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: cvi.Name,
				},
			})
		}
	}

	return
}
