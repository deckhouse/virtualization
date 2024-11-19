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
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type VirtualImageRequestEnqueuer struct {
	enqueueFromObj  client.Object
	enqueueFromKind virtv2.VirtualImageObjectRefKind
	client          client.Client
	logger          *log.Logger
}

func NewVirtualImageRequestEnqueuer(client client.Client, enqueueFromObj client.Object, enqueueFromKind virtv2.VirtualImageObjectRefKind) *VirtualImageRequestEnqueuer {
	return &VirtualImageRequestEnqueuer{
		enqueueFromObj:  enqueueFromObj,
		enqueueFromKind: enqueueFromKind,
		client:          client,
		logger:          log.Default().With("enqueuer", "vi"),
	}
}

func (w VirtualImageRequestEnqueuer) GetEnqueueFrom() client.Object {
	return w.enqueueFromObj
}

func (w VirtualImageRequestEnqueuer) EnqueueRequests(ctx context.Context, obj client.Object) (requests []reconcile.Request) {
	var vis virtv2.VirtualImageList
	err := w.client.List(ctx, &vis)
	if err != nil {
		w.logger.Error(fmt.Sprintf("failed to list vi: %s", err))
		return
	}

	for _, vi := range vis.Items {
		dsReady, _ := conditions.GetCondition(vicondition.DatasourceReadyType, vi.Status.Conditions)
		if dsReady.Status == metav1.ConditionTrue {
			continue
		}

		if vi.Spec.DataSource.Type != virtv2.DataSourceTypeObjectRef {
			continue
		}

		ref := vi.Spec.DataSource.ObjectRef

		if ref == nil || ref.Kind != w.enqueueFromKind {
			continue
		}

		if ref.Name == obj.GetName() {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: vi.Namespace,
					Name:      vi.Name,
				},
			})
		}
	}

	return
}
