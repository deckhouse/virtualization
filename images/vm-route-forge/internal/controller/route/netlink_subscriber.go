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

package route

import (
	"context"
	"fmt"
	"net"

	"github.com/go-logr/logr"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
	"k8s.io/apimachinery/pkg/types"

	vmipcache "vm-route-forge/internal/cache"
	"vm-route-forge/internal/netlinkwrap"
)

func NewNetlinkSubscriberWatcher(cidrs []*net.IPNet, cache vmipcache.Cache, nlWrapper *netlinkwrap.Funcs, log logr.Logger) *NetlinkSubscriberWatcher {
	return &NetlinkSubscriberWatcher{
		ch:       make(chan types.NamespacedName, defaultChanSize),
		cidrs:    cidrs,
		cache:    cache,
		log:      log.WithValues("watcher", NetlinkSubscriberKind),
		routeGet: nlWrapper.RouteGet,
	}
}

type NetlinkSubscriberWatcher struct {
	ch       chan types.NamespacedName
	cidrs    []*net.IPNet
	cache    vmipcache.Cache
	log      logr.Logger
	routeGet func(net.IP) ([]netlink.Route, error)
}

func (w *NetlinkSubscriberWatcher) Watch(ctx context.Context) (<-chan types.NamespacedName, error) {
	routeCh := make(chan netlink.RouteUpdate)
	if err := netlink.RouteSubscribe(routeCh, ctx.Done()); err != nil {
		return nil, fmt.Errorf("failed to subscribe to route updates: %w", err)
	}
	go func() {
		for ru := range routeCh {
			if err := w.sync(ru); err != nil {
				w.log.Error(err, "failed to sync route update")
			}
		}
	}()
	return w.ch, nil
}

// The cache is the source of truth.
// It contains all relevant information about the cluster,
// including the name and namespace of the virtual machine, its ip and ip nodes.
// We monitor updates in the routes and if we find a mismatch with the cache,
// we put the virtual machine in the queue for processing.
func (w *NetlinkSubscriberWatcher) sync(ru netlink.RouteUpdate) error {
	vmIP := ru.Dst.IP
	if vmIP == nil {
		return nil
	}
	isManaged, err := isManagedIP(vmIP, w.cidrs)
	if err != nil {
		return err
	}
	if !isManaged {
		return nil
	}
	ciliumInternalIP := ru.Src

	w.log.V(7).Info("Got new RouteUpdate", "value", ru)

	key, found := w.cache.GetName(vmIP)
	// if the route was added but not added to cache, then do nothing, because we can't get name of vm.
	if !found {
		return nil
	}

	log := w.log.WithValues(
		"ciliumInternalIP", ciliumInternalIP,
		"inHostVMIP", vmIP.String(),
		"virtualMachine", key)
	log.Info("Started processing route")

	switch ru.Type {
	case unix.RTM_NEWROUTE:
		addrs, found := w.cache.GetAddresses(key)
		if !found {
			log.Info("The route was added, but there is no addresses in the cache. Add the VM to the queue.")
			w.enqueueKey(key)
			break
		}
		routes, err := w.routeGet(addrs.NodeIP.NetIP())
		if err != nil || len(routes) == 0 {
			return fmt.Errorf("failed to get routes: %w", err)
		}
		ciliumInternalIPByNodeIP := routes[0].Src

		if !ciliumInternalIP.Equal(ciliumInternalIPByNodeIP) || !addrs.VMIP.NetIP().Equal(vmIP) {
			log.Info("The route was added, but the addresses from the cache and from the route do not match. Add the VM to the queue.",
				"inCacheNodeIP", addrs.NodeIP.String(),
				"inCacheVMIP", addrs.VMIP.String(),
				"ciliumInternalIPByNodeIP", ciliumInternalIPByNodeIP.String(),
			)
			w.enqueueKey(key)
		}
	case unix.RTM_DELROUTE:
		log.Info("The route was deleted but not deleted from the cache. Add the VM to the queue.")
		w.enqueueKey(key)
	}
	return nil
}

func (w *NetlinkSubscriberWatcher) enqueueKey(key types.NamespacedName) {
	w.ch <- key
}
