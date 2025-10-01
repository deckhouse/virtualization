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
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	_ "k8s.io/kubectl/pkg/drain"

	"github.com/deckhouse/virtualization/shatal/internal/api"
)

// Drainer does node drain sequentially in ascending order to push eviction of virtual machines.
type Drainer struct {
	api             *api.Client
	once            bool
	interval        time.Duration
	nodes           map[string]*sync.Mutex
	sortedNodes     []string
	lastDrainedNode string
	logger          *slog.Logger
}

func NewDrainer(api *api.Client, interval time.Duration, once bool, nodes map[string]*sync.Mutex, log *slog.Logger) *Drainer {
	sortedNodes := make([]string, 0, len(nodes))
	for n := range nodes {
		sortedNodes = append(sortedNodes, n)
	}

	return &Drainer{
		api:         api,
		interval:    interval,
		nodes:       nodes,
		once:        once,
		sortedNodes: sortedNodes,
		logger:      log.With("type", "drainer"),
	}
}

func (s *Drainer) Run(ctx context.Context) {
	slices.Sort(s.sortedNodes)

	node, err := s.getNodeName()
	if err != nil {
		s.logger.Error(err.Error())
		return
	}

	if s.once {
		s.drainNode(ctx, node)
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
			node, err = s.getNodeName()
			if err != nil {
				s.logger.Error(err.Error())
				return
			}

			s.drainNode(ctx, node)

			select {
			case <-time.After(s.interval):
			case <-ctx.Done():
				return
			}
		}
	}
}

func (s *Drainer) getNodeName() (string, error) {
	for i := 0; i < len(s.sortedNodes); i++ {
		if i == len(s.sortedNodes)-1 {
			return s.sortedNodes[0], nil
		}

		if s.sortedNodes[i] == s.lastDrainedNode {
			return s.sortedNodes[i+1], nil
		}
	}

	return "", errors.New("no nodes to drain")
}

func (s *Drainer) drainNode(ctx context.Context, node string) {
	s.nodes[node].Lock()
	s.logger.Info(fmt.Sprintf("Drain: %s", node))

	err := s.api.CordonNode(ctx, node)
	if err != nil {
		s.logger.Error(err.Error())
		return
	}

	err = s.api.DrainNode(ctx, node)
	if err != nil {
		s.logger.Error(err.Error())
		return
	}

	err = s.api.UnCordonNode(ctx, node)
	if err != nil {
		s.logger.Error(err.Error())
		return
	}

	vms, err := s.api.GetVMs(ctx)
	if err != nil {
		s.logger.Error(err.Error())
		return
	}

	for _, vm := range vms {
		if vm.Status.Node == node {
			panic(fmt.Sprintf("vm %s %s %s %s", vm.Name, vm.Namespace, vm.Status.Phase, vm.Status.Node))
		}
	}

	s.lastDrainedNode = node
	s.nodes[node].Unlock()
}
