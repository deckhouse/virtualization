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

	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	dvcrtypes "github.com/deckhouse/virtualization-controller/pkg/controller/dvcr-garbage-collection/types"
)

type DVCRDeploymentWatcher struct {
	client client.Client
}

func NewDVCRDeploymentWatcher(client client.Client) *DVCRDeploymentWatcher {
	return &DVCRDeploymentWatcher{
		client: client,
	}
}

// Watch adds watching for Deployment/dvcr changes.
func (w *DVCRDeploymentWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	return ctr.Watch(
		source.Kind(
			mgr.GetCache(),
			&appsv1.Deployment{},
			handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context, deploy *appsv1.Deployment) []reconcile.Request {
				if deploy.GetNamespace() == dvcrtypes.ModuleNamespace && deploy.GetName() == dvcrtypes.DVCRDeploymentName {
					return []reconcile.Request{{client.ObjectKeyFromObject(deploy)}}
				}
				return nil
			}),
			predicate.TypedFuncs[*appsv1.Deployment]{
				UpdateFunc: func(e event.TypedUpdateEvent[*appsv1.Deployment]) bool {
					return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration()
				},
			},
		),
	)
}
