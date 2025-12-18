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

package informer

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

const (
	PoolIndex   = "pool"
	DriverIndex = "driver"
)

func NewFactory(clientSet kubernetes.Interface, resync *time.Duration) *Factory {
	var defaultResync time.Duration
	if resync != nil {
		defaultResync = *resync
	} else {
		defaultResync = resyncPeriod(12 * time.Hour)
	}

	return &Factory{
		clientSet:        clientSet,
		defaultResync:    defaultResync,
		informers:        make(map[string]cache.SharedIndexInformer),
		startedInformers: make(map[string]struct{}),
	}
}

type Factory struct {
	clientSet     kubernetes.Interface
	defaultResync time.Duration

	informers        map[string]cache.SharedIndexInformer
	startedInformers map[string]struct{}
	mu               sync.Mutex
}

func (f *Factory) Run(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	group, ctx := errgroup.WithContext(ctx)

	for name, informer := range f.informers {
		if _, found := f.startedInformers[name]; found {
			// skip informers that have already started.
			slog.Info("SKIPPING informer", slog.String("name", name))
			continue
		}
		slog.Info("STARTING informer", slog.String("name", name))
		group.Go(func() error {
			informer.Run(ctx.Done())
			return nil
		})
		f.startedInformers[name] = struct{}{}
	}

	return group.Wait()
}

func (f *Factory) WaitForCacheSync(stopCh <-chan struct{}) {
	var syncs []cache.InformerSynced

	f.mu.Lock()
	for name, informer := range f.informers {
		slog.Info("Waiting for cache sync of informer", slog.String("name", name))
		syncs = append(syncs, informer.HasSynced)
	}
	f.mu.Unlock()

	cache.WaitForCacheSync(stopCh, syncs...)
}

func (f *Factory) ResourceSlice() cache.SharedIndexInformer {
	return f.getInformer("resourceSliceInformer", func() cache.SharedIndexInformer {
		lw := cache.NewListWatchFromClient(f.clientSet.ResourceV1().RESTClient(), "resourceslices", corev1.NamespaceAll, fields.Everything())
		return cache.NewSharedIndexInformer(lw, &resourcev1.ResourceSlice{}, f.defaultResync, cache.Indexers{
			PoolIndex: func(obj interface{}) ([]string, error) {
				return []string{obj.(*resourcev1.ResourceSlice).Spec.Pool.Name}, nil
			},
			DriverIndex: func(obj interface{}) ([]string, error) {
				return []string{obj.(*resourcev1.ResourceSlice).Spec.Driver}, nil
			},
		})
	})
}

func (f *Factory) NamespacedSecret(namespace string) cache.SharedIndexInformer {
	return f.getInformer("namespacedSecretInformer", func() cache.SharedIndexInformer {
		lw := cache.NewListWatchFromClient(f.clientSet.CoreV1().RESTClient(), "secrets", namespace, fields.Everything())
		return cache.NewSharedIndexInformer(lw, &corev1.Secret{}, f.defaultResync, cache.Indexers{})
	})
}

func (f *Factory) getInformer(key string, newFunc func() cache.SharedIndexInformer) cache.SharedIndexInformer {
	f.mu.Lock()
	defer f.mu.Unlock()

	informer, ok := f.informers[key]
	if ok {
		return informer
	}

	informer = newFunc()
	f.informers[key] = informer

	return informer
}

// resyncPeriod computes the time interval a shared informer waits before resyncing with the api server
func resyncPeriod(minResyncPeriod time.Duration) time.Duration {
	factor := rand.Float64() + 1
	return time.Duration(float64(minResyncPeriod.Nanoseconds()) * factor)
}
