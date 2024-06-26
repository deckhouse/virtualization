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
	"log/slog"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type VirtualDiskRequestEnqueuer struct {
	enqueueFromObj  client.Object
	enqueueFromKind virtv2.VirtualDiskObjectRefKind
	client          client.Client
	logger          *slog.Logger
}

func NewVirtualDiskRequestEnqueuer(client client.Client, enqueueFromObj client.Object, enqueueFromKind virtv2.VirtualDiskObjectRefKind) *VirtualDiskRequestEnqueuer {
	return &VirtualDiskRequestEnqueuer{
		enqueueFromObj:  enqueueFromObj,
		enqueueFromKind: enqueueFromKind,
		client:          client,
		logger:          slog.Default().With("enqueuer", "vd"),
	}
}

func (w VirtualDiskRequestEnqueuer) GetEnqueueFrom() client.Object {
	return w.enqueueFromObj
}

func (w VirtualDiskRequestEnqueuer) EnqueueRequests(ctx context.Context, obj client.Object) (requests []reconcile.Request) {
	var vds virtv2.VirtualDiskList
	err := w.client.List(ctx, &vds)
	if err != nil {
		w.logger.Error(fmt.Sprintf("failed to list vd: %s", err))
		return
	}

	for _, vd := range vds.Items {
		dsReady, _ := service.GetCondition(vdcondition.DatasourceReadyType, vd.Status.Conditions)
		if dsReady.Status == metav1.ConditionTrue {
			continue
		}

		if vd.Spec.DataSource == nil || vd.Spec.DataSource.Type != virtv2.DataSourceTypeObjectRef {
			continue
		}

		ref := vd.Spec.DataSource.ObjectRef

		if ref == nil || ref.Kind != w.enqueueFromKind {
			continue
		}

		if ref.Name == obj.GetName() {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: vd.Namespace,
					Name:      vd.Name,
				},
			})
		}
	}

	return
}
