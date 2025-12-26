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
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	resourcev1beta1 "k8s.io/api/resource/v1beta1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

const (
	NodeIndex   = "node"
	PoolIndex   = "pool"
	DriverIndex = "driver"
)

func NewFactory(clientSet *kubernetes.Clientset, resync *time.Duration) *Factory {
	var defaultResync time.Duration
	if resync != nil {
		defaultResync = *resync
	} else {
		defaultResync = resyncPeriod(12 * time.Hour)
	}

	return &Factory{
		clientSet:     clientSet,
		defaultResync: defaultResync,
		informers:     make(map[string]cache.SharedIndexInformer),
	}
}

type Factory struct {
	clientSet     *kubernetes.Clientset
	defaultResync time.Duration

	informers        map[string]cache.SharedIndexInformer
	startedInformers map[string]struct{}
	mu               sync.Mutex
}

func (f *Factory) Start(stopCh <-chan struct{}) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for name, informer := range f.informers {
		if _, found := f.startedInformers[name]; found {
			// skip informers that have already started.
			slog.Info("SKIPPING informer", slog.String("name", name))
			continue
		}
		slog.Info("STARTING informer", slog.String("name", name))
		go informer.Run(stopCh)
		f.startedInformers[name] = struct{}{}
	}
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

func (f *Factory) ResourceClaim() cache.SharedIndexInformer {
	return f.getInformer("resourceClaimInformer", func() cache.SharedIndexInformer {
		lw := cache.NewListWatchFromClient(f.clientSet.ResourceV1beta1().RESTClient(), "resourceclaims", corev1.NamespaceAll, fields.Everything())
		return cache.NewSharedIndexInformer(lw, &resourcev1beta1.ResourceClaim{}, f.defaultResync, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
	})
}

func (f *Factory) ResourceSlice() cache.SharedIndexInformer {
	return f.getInformer("resourceSliceInformer", func() cache.SharedIndexInformer {
		lw := cache.NewListWatchFromClient(f.clientSet.ResourceV1beta1().RESTClient(), "resourceslices", corev1.NamespaceAll, fields.Everything())
		return cache.NewSharedIndexInformer(lw, &resourcev1beta1.ResourceSlice{}, f.defaultResync, cache.Indexers{
			PoolIndex: func(obj interface{}) ([]string, error) {
				return []string{obj.(*resourcev1beta1.ResourceSlice).Spec.Pool.Name}, nil
			},
			DriverIndex: func(obj interface{}) ([]string, error) {
				return []string{obj.(*resourcev1beta1.ResourceSlice).Spec.Driver}, nil
			},
		})
	})
}

func (f *Factory) Nodes() cache.SharedIndexInformer {
	return f.getInformer("nodesInformer", func() cache.SharedIndexInformer {
		lw := cache.NewListWatchFromClient(f.clientSet.CoreV1().RESTClient(), "nodes", corev1.NamespaceAll, fields.Everything())
		return cache.NewSharedIndexInformer(lw, &corev1.Node{}, f.defaultResync, cache.Indexers{})
	})
}

func (f *Factory) Pods() cache.SharedIndexInformer {
	return f.getInformer("podsInformer", func() cache.SharedIndexInformer {
		lw := cache.NewListWatchFromClient(f.clientSet.CoreV1().RESTClient(), "pods", corev1.NamespaceAll, fields.Everything())
		return cache.NewSharedIndexInformer(lw, &corev1.Pod{}, f.defaultResync, cache.Indexers{
			cache.NamespaceIndex: cache.MetaNamespaceIndexFunc,
			NodeIndex: func(obj interface{}) ([]string, error) {
				return []string{obj.(*corev1.Pod).Spec.NodeName}, nil
			},
		})
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
