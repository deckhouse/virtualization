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

	"vm-route-forge/internal/cache"
)

const (
	defaultChanSize = 100
)

type Watcher interface {
	Watch(ctx context.Context) (chan types.NamespacedName, error)
}

type KindRouteWatcher string

const (
	NetlinkKind KindRouteWatcher = "netlink"
	TickerKind  KindRouteWatcher = "ticker"
	EbpfKind    KindRouteWatcher = "ebpf"
)

func WatchFactory(kind KindRouteWatcher,
	cidrs []*net.IPNet,
	cache cache.Cache,
	tableID int,
	log logr.Logger,
) (Watcher, error) {
	switch kind {
	case NetlinkKind:
		return NewNetlinkWatcher(cidrs, cache, log), nil
	case TickerKind:
		return NewTickerWatcher(cidrs, cache, tableID, log), nil
	case EbpfKind:
		return NewEbpfWatcher(cidrs, cache, log)
	default:
		return nil, fmt.Errorf("unknown kind %s", kind)
	}
}
