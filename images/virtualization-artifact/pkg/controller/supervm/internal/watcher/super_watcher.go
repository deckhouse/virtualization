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
	"fmt"

	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func NewNamespacedWatcher() *NamespacedWatcher {
	return &NamespacedWatcher{}
}

type NamespacedWatcher struct{}

func (w *NamespacedWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	err := ctr.Watch(&NamespacedSource{})
	if err != nil {
		return fmt.Errorf("error setting watch on NamespacedSource: %w", err)
	}
	return nil
}

type NamespacedSource struct {
	queue <-chan reconcile.Request
}

func (s *NamespacedSource) Start(ctx context.Context, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) error {
	go func() {
		select {
		case req, ok := <-s.queue:
			if !ok {
				return
			}
			queue.Add(req)
		case <-ctx.Done():
			return
		}
	}()

	return nil
}
