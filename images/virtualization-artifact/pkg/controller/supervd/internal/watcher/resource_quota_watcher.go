/*
Copyright 2025 Flant JSC

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

package watcher

import (
	"context"
	"fmt"
	log "log/slog"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type ResourceQuotaWatcher struct {
	logger *log.Logger
	client client.Client
}

func NewResourceQuotaWatcher(client client.Client) *ResourceQuotaWatcher {
	return &ResourceQuotaWatcher{
		logger: log.Default().With("watcher", strings.ToLower("ResourceQuota")),
		client: client,
	}
}

func (w ResourceQuotaWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &corev1.ResourceQuota{},
			handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context, quota *corev1.ResourceQuota) []reconcile.Request {
				return w.enqueueRequests(ctx, quota)
			}),
			predicate.TypedFuncs[*corev1.ResourceQuota]{
				CreateFunc: func(e event.TypedCreateEvent[*corev1.ResourceQuota]) bool {
					return false
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on ResourceQuota: %w", err)
	}
	return nil
}

func (w ResourceQuotaWatcher) enqueueRequests(ctx context.Context, obj client.Object) (requests []reconcile.Request) {
	var vds v1alpha2.VirtualDiskList
	err := w.client.List(ctx, &vds, client.InNamespace(obj.GetNamespace()))
	if err != nil {
		w.logger.Error(fmt.Sprintf("failed to get virtual disks: %s", err))
		return
	}

	for _, vd := range vds.Items {
		readyCondition, _ := conditions.GetCondition(vdcondition.ReadyType, vd.Status.Conditions)
		if readyCondition.Reason == vdcondition.QuotaExceeded.String() {
			requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{
				Namespace: vd.Namespace,
				Name:      vd.Name,
			}})
		}
	}

	return
}
