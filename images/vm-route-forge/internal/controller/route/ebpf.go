package route

import (
	"context"
	"fmt"
	"net"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"

	"vm-route-forge/internal/cache"
)

func NewEbpfWatcher(cidrs []*net.IPNet, cache cache.Cache, log logr.Logger) (*EbpfWatcher, error) {
	return &EbpfWatcher{}, fmt.Errorf("not implemented")
}

type EbpfWatcher struct {
}

func (w *EbpfWatcher) Watch(ctx context.Context) (chan types.NamespacedName, error) {
	return nil, nil
}
