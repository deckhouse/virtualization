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
	"k8s.io/client-go/util/workqueue"

	"vm-route-forge/internal/cache"
	netlinkwrap2 "vm-route-forge/internal/netlinkwrap"
)

func NewHostController(queue workqueue.RateLimitingInterface, cidrs []*net.IPNet, cache cache.Cache, log logr.Logger) *HostRouteController {
	return &HostRouteController{
		queue:    queue,
		cidrs:    cidrs,
		cache:    cache,
		log:      log,
		routeGet: netlinkwrap2.NewFuncs().RouteGet,
	}
}

type HostRouteController struct {
	queue    workqueue.RateLimitingInterface
	cidrs    []*net.IPNet
	cache    cache.Cache
	log      logr.Logger
	routeGet func(net.IP) ([]netlink.Route, error)
}

func (r *HostRouteController) Run(ctx context.Context) error {
	ch := make(chan netlink.RouteUpdate)
	if err := netlink.RouteSubscribe(ch, ctx.Done()); err != nil {
		return fmt.Errorf("failed to subscribe to route updates: %w", err)
	}
	for ru := range ch {
		if err := r.sync(ru); err != nil {
			r.log.Error(err, "failed to sync route update")
		}
	}
	return nil
}

// The cache is the source of truth.
// It contains all relevant information about the cluster,
// including the name and namespace of the virtual machine, its ip and ip nodes.
// We monitor updates in the routes and if we find a mismatch with the cache,
// we put the virtual machine in the queue for processing.
func (r *HostRouteController) sync(ru netlink.RouteUpdate) error {
	vmIP := ru.Dst.IP
	if vmIP == nil {
		return nil
	}
	isManaged, err := r.isManagedIP(vmIP)
	if err != nil {
		return err
	}
	if !isManaged {
		return nil
	}
	ciliumInternalIP := ru.Src

	r.log.V(7).Info("Got new RouteUpdate", "value", ru)

	key, found := r.cache.GetName(vmIP)

	log := r.log.WithValues(
		"ciliumInternalIP", ciliumInternalIP,
		"inHostVMIP", vmIP.String(),
		"virtualMachine", key)
	log.Info("Started processing route")

	switch ru.Type {
	case unix.RTM_NEWROUTE:
		// if the route was added but not added to cache, then do nothing, because we can't get name of vm.
		if !found {
			break
		}
		addrs, found := r.cache.GetAddresses(key)
		if !found {
			log.Info("The route was added, but there is no addresses in the cache. Add the VM to the queue.")
			r.enqueueKey(key)
			break
		}
		routes, err := r.routeGet(addrs.NodeIP)
		if err != nil || len(routes) == 0 {
			return fmt.Errorf("failed to get routes: %w", err)
		}
		ciliumInternalIPByNodeIP := routes[0].Src

		if !ciliumInternalIP.Equal(ciliumInternalIPByNodeIP) || !addrs.VMIP.Equal(vmIP) {
			log.Info("The route was added, but the addresses from the cache and from the route do not match. Add the VM to the queue.",
				"inCacheNodeIP", addrs.NodeIP.String(),
				"inCacheVMIP", addrs.VMIP.String(),
				"ciliumInternalIPByNodeIP", ciliumInternalIPByNodeIP.String(),
			)
			r.enqueueKey(key)
		}
	case unix.RTM_DELROUTE:
		if found {
			log.Info("The route was deleted but not deleted from the cache. Add the VM to the queue.")
			r.enqueueKey(key)
		}
	}
	return nil
}

func (r *HostRouteController) isManagedIP(ip net.IP) (bool, error) {
	if len(ip) == 0 {
		return false, fmt.Errorf("invalid IP address %s", ip)
	}

	for _, cidr := range r.cidrs {
		if cidr.Contains(ip) {
			return true, nil
		}
	}

	return false, nil
}

func (r *HostRouteController) enqueueKey(key types.NamespacedName) {
	r.queue.Add(key.String())
}
