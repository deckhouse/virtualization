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
)

func NewHostController(queue workqueue.RateLimitingInterface, cidrs []*net.IPNet, cache cache.Cache, log *logr.Logger) *HostRouteController {
	return &HostRouteController{
		queue: queue,
		cidrs: cidrs,
		cache: cache,
		log:   log,
	}
}

type HostRouteController struct {
	queue workqueue.RateLimitingInterface
	cidrs []*net.IPNet
	cache cache.Cache
	log   *logr.Logger
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

func (r *HostRouteController) sync(ru netlink.RouteUpdate) error {
	dst := ru.Dst
	if dst == nil {
		return nil
	}
	isManaged, err := r.isManagedIP(dst.IP)
	if err != nil {
		return err
	}
	if !isManaged {
		return nil
	}
	src := ru.Src

	key, found := r.cache.GetName(dst.IP.String())
	switch ru.Type {
	case unix.RTM_NEWROUTE:
		if !found {
			r.enqueueKey(key)
		}
		addrs, found := r.cache.GetAddresses(key)
		if !found {
			r.enqueueKey(key)
		}
		if addrs.NodeIP != src.String() || addrs.VMIP != dst.String() {
			r.enqueueKey(key)
		}
	case unix.RTM_DELROUTE:
		if found {
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
