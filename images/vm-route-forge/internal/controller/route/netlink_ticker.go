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
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/vishvananda/netlink"
	"k8s.io/apimachinery/pkg/types"

	vmipcache "vm-route-forge/internal/cache"
	"vm-route-forge/internal/netlinkwrap"
)

func NewNetlinkTickerWatcher(ctx context.Context,
	cidrs []*net.IPNet,
	cache vmipcache.Cache,
	routeTableID int,
	nlWrapper *netlinkwrap.Funcs,
	log logr.Logger,
) *NetlinkTickerWatcher {
	ctx, cancel := context.WithCancel(ctx)
	w := &NetlinkTickerWatcher{
		ctx:          ctx,
		cancel:       cancel,
		result:       make(chan types.NamespacedName, defaultChanSize),
		cidrs:        cidrs,
		cache:        cache,
		routeTableID: routeTableID,
		log:          log.WithValues("watcher", NetlinkTickerKind),
		nlWrapper:    nlWrapper,
	}
	go w.watch()
	return w
}

type NetlinkTickerWatcher struct {
	sync.Mutex
	ctx          context.Context
	cancel       context.CancelFunc
	result       chan types.NamespacedName
	cidrs        []*net.IPNet
	cache        vmipcache.Cache
	routeTableID int
	log          logr.Logger
	nlWrapper    *netlinkwrap.Funcs
}

func (w *NetlinkTickerWatcher) ResultChannel() <-chan types.NamespacedName {
	return w.result
}

func (w *NetlinkTickerWatcher) Stop() {
	w.Lock()
	defer w.Unlock()
	select {
	case <-w.ctx.Done():
	default:
		w.cancel()
	}
}

func (w *NetlinkTickerWatcher) watch() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	defer close(w.result)
	defer w.Stop()
	for {
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			if err := w.sync(); err != nil {
				w.log.Error(err, "failed to sync routes")
			}
		}
	}
}

func (w *NetlinkTickerWatcher) sync() error {
	routes, err := w.nlWrapper.RouteListFiltered(netlink.FAMILY_ALL, &netlink.Route{Table: w.routeTableID}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return fmt.Errorf("failed to list routes: %w", err)
	}
	routeMap := make(map[string]*netlink.Route, len(routes))
	for _, route := range routes {
		routeMap[route.Dst.IP.String()] = &route
	}

	// enqueue vm with missing routes
	w.cache.Iterate(func(k types.NamespacedName, v vmipcache.Addresses) (next bool) {
		if _, found := routeMap[v.VMIP.String()]; !found {
			w.log.Info(fmt.Sprintf("Missing route. Add the VM %q to the queue.", k))
			w.enqueueKey(k)
		}
		return true
	})

	for vmIP, route := range routeMap {
		if err = w.syncRoute(route); err != nil {
			w.log.Error(err, "failed to sync route", "vmIP", vmIP)
		}
	}
	return nil
}

func (w *NetlinkTickerWatcher) syncRoute(route *netlink.Route) error {
	if route == nil {
		return nil
	}
	vmIP := route.Dst.IP
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
	ciliumInternalIP := route.Src
	key, found := w.cache.GetName(vmIP)
	// if the route was added but not added to cache, then do nothing, because we can't get name of vm.
	if !found {
		return nil
	}

	log := w.log.WithValues(
		"ciliumInternalIP", ciliumInternalIP,
		"inHostVMIP", vmIP.String(),
		"virtualMachine", key)

	addrs, found := w.cache.GetAddresses(key)
	if !found {
		log.Info("The route was added, but there are no addresses in the cache. Add the VM to the queue.")
		w.enqueueKey(key)
		return nil
	}
	routes, err := w.nlWrapper.RouteGet(addrs.NodeIP.NetIP())
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
	return nil
}

func (w *NetlinkTickerWatcher) enqueueKey(key types.NamespacedName) {
	w.result <- key
}
