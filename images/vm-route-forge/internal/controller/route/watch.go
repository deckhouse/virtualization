package route

import (
	"context"
	"fmt"
	"net"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"

	"vm-route-forge/internal/cache"
)

type Watcher interface {
	Watch(ctx context.Context) (chan types.NamespacedName, error)
}

type KindRouteWatcher string

const (
	NetlinkKind KindRouteWatcher = "netlink"
)

func WatchFactory(kind KindRouteWatcher, cidrs []*net.IPNet, cache cache.Cache, log logr.Logger) (Watcher, error) {
	switch kind {
	case NetlinkKind:
		return NewNetlinkWatcher(cidrs, cache, log), nil
	default:
		return nil, fmt.Errorf("unknown kind %s", kind)
	}
}
