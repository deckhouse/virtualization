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

package shatal

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"math/big"
	"sync"
	"time"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/shatal/internal/api"
)

type Doer interface {
	Do(ctx context.Context, vm v1alpha2.VirtualMachine)
}

type Watcher struct {
	api      *api.Client
	interval time.Duration
	nodes    map[string]*sync.Mutex
	vmSubs   []Doer
	logger   *slog.Logger
}

func NewWatcher(api *api.Client, interval time.Duration, nodes map[string]*sync.Mutex, log *slog.Logger) *Watcher {
	return &Watcher{
		api:      api,
		interval: interval,
		nodes:    nodes,
		logger:   log,
	}
}

func (c *Watcher) Run(ctx context.Context) {
	for {
		if len(c.vmSubs) > 0 {
			c.run(ctx)
		}

		select {
		case <-time.After(c.interval):
		case <-ctx.Done():
			c.logger.Info("API Watcher stopped")
			return
		}
	}
}

func (c *Watcher) Subscribe(doer Doer, weight int) {
	for i := 0; i < weight; i++ {
		c.vmSubs = append(c.vmSubs, doer)
	}
}

func (c *Watcher) run(ctx context.Context) {
	vms, err := c.api.GetVMs(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}

		panic(err)
	}

	for _, vm := range vms {
		if vm.Status.Phase != v1alpha2.MachineRunning || vm.DeletionTimestamp != nil || vm.Status.Node == "" {
			continue
		}

		node, ok := c.nodes[vm.Status.Node]
		if ok && !node.TryLock() {
			continue
		}

		subsLen := big.NewInt(int64(len(c.vmSubs)))

		randomSub, err := rand.Int(rand.Reader, subsLen)
		if err != nil {
			panic(err)
		}

		c.vmSubs[randomSub.Int64()].Do(ctx, vm)
		if ok {
			node.Unlock()
		}
	}
}
