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
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type VirtualDiskRequestEnqueuer struct {
	enqueueFromObj  client.Object
	enqueueFromKind v1alpha2.VirtualDiskObjectRefKind
	client          client.Client
	logger          *log.Logger
}

func NewVirtualDiskRequestEnqueuer(client client.Client, enqueueFromObj client.Object, enqueueFromKind v1alpha2.VirtualDiskObjectRefKind) *VirtualDiskRequestEnqueuer {
	return &VirtualDiskRequestEnqueuer{
		enqueueFromObj:  enqueueFromObj,
		enqueueFromKind: enqueueFromKind,
		client:          client,
		logger:          log.Default().With("enqueuer", "vd"),
	}
}

func (w VirtualDiskRequestEnqueuer) GetEnqueueFrom() client.Object {
	return w.enqueueFromObj
}

func (w VirtualDiskRequestEnqueuer) EnqueueRequestsFromVDs(ctx context.Context, obj client.Object) (requests []reconcile.Request) {
	var vds v1alpha2.VirtualDiskList
	err := w.client.List(ctx, &vds)
	if err != nil {
		w.logger.Error(fmt.Sprintf("failed to list vd: %s", err))
		return
	}

	for _, vd := range vds.Items {
		dsReady, _ := conditions.GetCondition(vdcondition.DatasourceReadyType, vd.Status.Conditions)
		ready, _ := conditions.GetCondition(vdcondition.ReadyType, vd.Status.Conditions)
		if ready.Status == metav1.ConditionTrue || dsReady.Status == metav1.ConditionTrue {
			continue
		}

		if vd.Spec.DataSource == nil || vd.Spec.DataSource.Type != v1alpha2.DataSourceTypeObjectRef {
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

func (w VirtualDiskRequestEnqueuer) EnqueueRequestsFromVIs(obj client.Object) (requests []reconcile.Request) {
	if w.enqueueFromKind == v1alpha2.VirtualDiskObjectRefKindVirtualImage {
		vi, ok := obj.(*v1alpha2.VirtualImage)
		if !ok {
			w.logger.Error(fmt.Sprintf("expected a VirtualImage but got a %T", obj))
			return
		}

		if vi.Spec.DataSource.Type == v1alpha2.DataSourceTypeObjectRef && vi.Spec.DataSource.ObjectRef != nil && vi.Spec.DataSource.ObjectRef.Kind == v1alpha2.VirtualDiskKind {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      vi.Spec.DataSource.ObjectRef.Name,
					Namespace: vi.Namespace,
				},
			})
		}
	}
	return
}

func (w VirtualDiskRequestEnqueuer) EnqueueRequestsFromCVIs(obj client.Object) (requests []reconcile.Request) {
	if w.enqueueFromKind == v1alpha2.VirtualDiskObjectRefKindClusterVirtualImage {
		cvi, ok := obj.(*v1alpha2.ClusterVirtualImage)
		if !ok {
			w.logger.Error(fmt.Sprintf("expected a ClusterVirtualImage but got a %T", obj))
			return
		}

		if cvi.Spec.DataSource.Type == v1alpha2.DataSourceTypeObjectRef && cvi.Spec.DataSource.ObjectRef != nil && cvi.Spec.DataSource.ObjectRef.Kind == v1alpha2.VirtualDiskKind {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      cvi.Spec.DataSource.ObjectRef.Name,
					Namespace: cvi.Spec.DataSource.ObjectRef.Namespace,
				},
			})
		}
	}
	return
}

func (w VirtualDiskRequestEnqueuer) EnqueueRequests(ctx context.Context, obj client.Object) (requests []reconcile.Request) {
	vds := w.EnqueueRequestsFromVDs(ctx, obj)
	vdsFromVIs := w.EnqueueRequestsFromVIs(obj)
	vdsFromCVIs := w.EnqueueRequestsFromCVIs(obj)

	uniqueRequests := make(map[reconcile.Request]struct{})

	for _, req := range vds {
		uniqueRequests[req] = struct{}{}
	}
	for _, req := range vdsFromVIs {
		uniqueRequests[req] = struct{}{}
	}
	for _, req := range vdsFromCVIs {
		uniqueRequests[req] = struct{}{}
	}

	var aggregatedResults []reconcile.Request
	for req := range uniqueRequests {
		aggregatedResults = append(aggregatedResults, req)
	}

	return aggregatedResults
}
