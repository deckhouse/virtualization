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
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

type DVCRDeploymentWatcher struct {
	client client.Client
}

func NewDVCRDeploymentWatcher(client client.Client) *DVCRDeploymentWatcher {
	return &DVCRDeploymentWatcher{
		client: client,
	}
}

// Watch adds watching for Deployment/dvcr changes and for cron events.
func (w *DVCRDeploymentWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(
			mgr.GetCache(),
			&appsv1.Deployment{},
			&handler.TypedEnqueueRequestForObject[*appsv1.Deployment]{},
			predicate.TypedFuncs[*appsv1.Deployment]{
				UpdateFunc: func(e event.TypedUpdateEvent[*appsv1.Deployment]) bool {
					return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration()
				},
			},
		),
	); err != nil {
		return err
	}

	return nil
}
