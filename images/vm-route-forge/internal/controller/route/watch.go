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
	"k8s.io/apimachinery/pkg/types"

	vmipcache "vm-route-forge/internal/cache"
	netlinkwrap "vm-route-forge/internal/netlinkwrap"
)

const (
	defaultChanSize = 100
)

type Watcher interface {
	ResultChannel() <-chan types.NamespacedName
	Stop()
}

type KindRouteWatcher string

const (
	NetlinkTickerKind KindRouteWatcher = "netlinkTicker"
	EbpfKind          KindRouteWatcher = "ebpf"
)

func WatchFactory(ctx context.Context,
	kind KindRouteWatcher,
	cidrs []*net.IPNet,
	cache vmipcache.Cache,
	routeTableID int,
	nlWrapper *netlinkwrap.Funcs,
	log logr.Logger,
) (Watcher, error) {
	switch kind {
	case NetlinkTickerKind:
		return NewNetlinkTickerWatcher(ctx, cidrs, cache, routeTableID, nlWrapper, log), nil
	case EbpfKind:
		return NewEbpfWatcher(ctx, cidrs, routeTableID, cache, nlWrapper, log)
	default:
		return nil, fmt.Errorf("unknown kind %s", kind)
	}
}
