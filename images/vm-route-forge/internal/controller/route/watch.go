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
