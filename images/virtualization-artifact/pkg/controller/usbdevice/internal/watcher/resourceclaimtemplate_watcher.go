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
	"reflect"
	"strings"

	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const resourceClaimTemplateNameSuffix = "-template"

func NewResourceClaimTemplateWatcher() *ResourceClaimTemplateWatcher {
	return &ResourceClaimTemplateWatcher{}
}

type ResourceClaimTemplateWatcher struct{}

func (w *ResourceClaimTemplateWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	return ctr.Watch(
		source.Kind(mgr.GetCache(),
			&resourcev1.ResourceClaimTemplate{},
			handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context, template *resourcev1.ResourceClaimTemplate) []reconcile.Request {
				_ = ctx

				name, ok := mapResourceClaimTemplateToUSBDeviceName(template)
				if !ok {
					return nil
				}

				return []reconcile.Request{{
					NamespacedName: types.NamespacedName{Namespace: template.Namespace, Name: name},
				}}
			}),
			predicate.TypedFuncs[*resourcev1.ResourceClaimTemplate]{
				CreateFunc: func(e event.TypedCreateEvent[*resourcev1.ResourceClaimTemplate]) bool {
					return e.Object != nil
				},
				DeleteFunc: func(e event.TypedDeleteEvent[*resourcev1.ResourceClaimTemplate]) bool {
					return e.Object != nil
				},
				UpdateFunc: func(e event.TypedUpdateEvent[*resourcev1.ResourceClaimTemplate]) bool {
					return shouldProcessResourceClaimTemplateUpdate(e.ObjectOld, e.ObjectNew)
				},
			},
		),
	)
}

func mapResourceClaimTemplateToUSBDeviceName(template *resourcev1.ResourceClaimTemplate) (string, bool) {
	if template == nil {
		return "", false
	}

	for _, ref := range template.OwnerReferences {
		if ref.Kind == v1alpha2.USBDeviceKind && ref.APIVersion == v1alpha2.SchemeGroupVersion.String() {
			return ref.Name, true
		}
	}

	if name, hasSuffix := strings.CutSuffix(template.Name, resourceClaimTemplateNameSuffix); hasSuffix && name != "" {
		return name, true
	}

	return "", false
}

func shouldProcessResourceClaimTemplateUpdate(oldObj, newObj *resourcev1.ResourceClaimTemplate) bool {
	if oldObj == nil || newObj == nil {
		return false
	}

	return !reflect.DeepEqual(oldObj.OwnerReferences, newObj.OwnerReferences) || !reflect.DeepEqual(oldObj.Spec, newObj.Spec)
}
