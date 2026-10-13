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

	"github.com/deckhouse/deckhouse/pkg/log"
	commonnetwork "github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization-controller/pkg/common/network/sdnapi"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// MainClusterNetworkWatcher wakes every vmip once the ClusterNetwork main gets a pool:
// the vmip reconcile creates the mirrors, and no IPAddress exists yet to send an event.
type MainClusterNetworkWatcher struct {
	client client.Client
	logger *log.Logger
}

func NewMainClusterNetworkWatcher(client client.Client) *MainClusterNetworkWatcher {
	return &MainClusterNetworkWatcher{
		client: client,
		logger: log.Default().With("watcher", "clusternetwork"),
	}
}

func (w MainClusterNetworkWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if !featuregates.Default().Enabled(featuregates.SDN) {
		return nil
	}
	if err := ctr.Watch(
		source.Kind(
			mgr.GetCache(),
			&sdnapi.ClusterNetwork{},
			handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context, _ *sdnapi.ClusterNetwork) []reconcile.Request {
				return w.enqueueAllVMIPs(ctx)
			}),
			predicate.TypedFuncs[*sdnapi.ClusterNetwork]{
				CreateFunc: func(e event.TypedCreateEvent[*sdnapi.ClusterNetwork]) bool {
					return mainNetworkHasPool(e.Object)
				},
				// Only the pool reference matters here: status churn on the network
				// would otherwise requeue every vmip in the cluster.
				UpdateFunc: func(e event.TypedUpdateEvent[*sdnapi.ClusterNetwork]) bool {
					return mainNetworkHasPool(e.ObjectNew) && poolName(e.ObjectOld) != poolName(e.ObjectNew)
				},
				DeleteFunc:  func(e event.TypedDeleteEvent[*sdnapi.ClusterNetwork]) bool { return false },
				GenericFunc: func(e event.TypedGenericEvent[*sdnapi.ClusterNetwork]) bool { return false },
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on ClusterNetwork: %w", err)
	}
	return nil
}

// mainNetworkHasPool is MainIPAMAvailable for the object of the event.
func mainNetworkHasPool(clusterNetwork *sdnapi.ClusterNetwork) bool {
	return clusterNetwork.GetName() == commonnetwork.MainClusterNetworkName &&
		clusterNetwork.Spec.Type == sdnapi.ClusterNetworkTypePrimaryCNI && poolName(clusterNetwork) != ""
}

func poolName(clusterNetwork *sdnapi.ClusterNetwork) string {
	if clusterNetwork.Spec.IPAM == nil || clusterNetwork.Spec.IPAM.IPAddressPoolRef == nil {
		return ""
	}
	return clusterNetwork.Spec.IPAM.IPAddressPoolRef.Name
}

func (w MainClusterNetworkWatcher) enqueueAllVMIPs(ctx context.Context) []reconcile.Request {
	var vmips v1alpha2.VirtualMachineIPAddressList
	if err := w.client.List(ctx, &vmips); err != nil {
		w.logger.Error("failed to list vmips", "err", err)
		return nil
	}
	requests := make([]reconcile.Request, 0, len(vmips.Items))
	for i := range vmips.Items {
		requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{
			Name:      vmips.Items[i].Name,
			Namespace: vmips.Items[i].Namespace,
		}})
	}
	return requests
}
