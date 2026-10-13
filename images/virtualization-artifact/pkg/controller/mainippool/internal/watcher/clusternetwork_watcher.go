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
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization-controller/pkg/common/network/sdnapi"
)

// ClusterNetworkWatcher wakes the controller on the main ClusterNetwork: the
// reference to the pool lives in it, and it is the only network this controller
// has business with.
type ClusterNetworkWatcher struct{}

func NewClusterNetworkWatcher() *ClusterNetworkWatcher {
	return &ClusterNetworkWatcher{}
}

func (w *ClusterNetworkWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	enqueue := handler.TypedEnqueueRequestsFromMapFunc(func(_ context.Context, _ *sdnapi.ClusterNetwork) []reconcile.Request {
		return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: network.MainIPAddressPoolName}}}
	})

	isMain := func(o client.Object) bool { return o.GetName() == network.MainClusterNetworkName }
	byMainNetwork := predicate.TypedFuncs[*sdnapi.ClusterNetwork]{
		CreateFunc:  func(e event.TypedCreateEvent[*sdnapi.ClusterNetwork]) bool { return isMain(e.Object) },
		UpdateFunc:  func(e event.TypedUpdateEvent[*sdnapi.ClusterNetwork]) bool { return isMain(e.ObjectNew) },
		DeleteFunc:  func(e event.TypedDeleteEvent[*sdnapi.ClusterNetwork]) bool { return isMain(e.Object) },
		GenericFunc: func(e event.TypedGenericEvent[*sdnapi.ClusterNetwork]) bool { return isMain(e.Object) },
	}

	if err := ctr.Watch(source.Kind(mgr.GetCache(), &sdnapi.ClusterNetwork{}, enqueue, byMainNetwork)); err != nil {
		return fmt.Errorf("error setting watch on ClusterNetwork: %w", err)
	}
	return nil
}
