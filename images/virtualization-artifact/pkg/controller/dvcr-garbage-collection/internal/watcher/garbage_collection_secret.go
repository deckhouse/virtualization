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
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	dvcrtypes "github.com/deckhouse/virtualization-controller/pkg/controller/dvcr-garbage-collection/types"
)

type DVCRGarbageCollectionSecretWatcher struct {
	client client.Client
}

func NewDVCRGarbageCollectionSecretWatcher(client client.Client) *DVCRGarbageCollectionSecretWatcher {
	return &DVCRGarbageCollectionSecretWatcher{
		client: client,
	}
}

// Watch adds watching for Deployment/dvcr changes and for cron events.
func (w *DVCRGarbageCollectionSecretWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(
			mgr.GetCache(),
			&corev1.Secret{},
			&handler.TypedEnqueueRequestForObject[*corev1.Secret]{},
			predicate.TypedFuncs[*corev1.Secret]{
				UpdateFunc: func(e event.TypedUpdateEvent[*corev1.Secret]) bool {
					// Handle only garbage collection secret.
					if e.ObjectOld.Namespace != dvcrtypes.ModuleNamespace {
						return false
					}
					if e.ObjectOld.Name != dvcrtypes.DVCRGarbageCollectionSecretName {
						return false
					}
					return !reflect.DeepEqual(e.ObjectNew.GetAnnotations(), e.ObjectOld.GetAnnotations())
				},
			},
		),
	); err != nil {
		return err
	}

	return nil
}
