/*
Copyright 2026 Flant JSC

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

	resourcev1beta1 "k8s.io/api/resource/v1beta1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func NewResourceClaimTemplateWatcher() *ResourceClaimTemplateWatcher {
	return &ResourceClaimTemplateWatcher{}
}

type ResourceClaimTemplateWatcher struct{}

func (w *ResourceClaimTemplateWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	return ctr.Watch(
		source.Kind(mgr.GetCache(),
			&resourcev1beta1.ResourceClaimTemplate{},
			handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context, template *resourcev1beta1.ResourceClaimTemplate) []reconcile.Request {
				for _, ref := range template.OwnerReferences {
					if ref.Kind == v1alpha2.USBDeviceKind && ref.APIVersion == v1alpha2.SchemeGroupVersion.String() {
						return []reconcile.Request{{
							NamespacedName: types.NamespacedName{
								Namespace: template.Namespace,
								Name:      ref.Name,
							},
						}}
					}
				}
				return nil
			}),
		),
	)
}
