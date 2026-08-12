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
	"fmt"

	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/component-base/featuregate"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func NewResourceClaimTemplateWatcher(featureGate featuregate.FeatureGate) *ResourceClaimTemplateWatcher {
	return &ResourceClaimTemplateWatcher{featureGate: featureGate}
}

type ResourceClaimTemplateWatcher struct {
	featureGate featuregate.FeatureGate
}

func (w *ResourceClaimTemplateWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	// ResourceClaimTemplate belongs to resource.k8s.io, an API group that reached v1 in Kubernetes
	// 1.34. Watching a kind the cluster does not serve never syncs the cache and brings the whole
	// manager down, so the watch is set only for the feature that owns these templates.
	if !w.featureGate.Enabled(featuregates.GPU) {
		return nil
	}

	if err := ctr.Watch(
		source.Kind(
			mgr.GetCache(),
			&resourcev1.ResourceClaimTemplate{},
			handler.TypedEnqueueRequestForOwner[*resourcev1.ResourceClaimTemplate](
				mgr.GetScheme(),
				mgr.GetRESTMapper(),
				&v1alpha2.VirtualMachine{},
				handler.OnlyControllerOwner(),
			),
		),
	); err != nil {
		return fmt.Errorf("error setting watch on ResourceClaimTemplate: %w", err)
	}
	return nil
}
