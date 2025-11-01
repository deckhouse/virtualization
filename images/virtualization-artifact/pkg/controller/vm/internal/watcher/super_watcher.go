/*
Copyright 2025 Flant JSC

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

package watcher

import (
	"context"

	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/logger"
)

type SuperSource struct {
	queue <-chan reconcile.Request
}

func NewSuperSource(queue <-chan reconcile.Request) *SuperSource {
	return &SuperSource{
		queue: queue,
	}
}

func (s *SuperSource) Start(ctx context.Context, wq workqueue.TypedRateLimitingInterface[reconcile.Request]) error {
	go func() {
		for req := range s.queue {
			logger.FromContext(ctx).Warn("[test][SOURCE] START")
			wq.Add(req)
			logger.FromContext(ctx).Warn("[test][SOURCE] FINISHED")
		}
	}()

	return nil
}
