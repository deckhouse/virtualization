/*
Copyright Flant JSC

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

// Code generated by informer-gen. DO NOT EDIT.

package v1alpha2

import (
	"context"
	time "time"

	versioned "github.com/deckhouse/virtualization/api/client/generated/clientset/versioned"
	internalinterfaces "github.com/deckhouse/virtualization/api/client/generated/informers/externalversions/internalinterfaces"
	v1alpha2 "github.com/deckhouse/virtualization/api/client/generated/listers/core/v1alpha2"
	corev1alpha2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	watch "k8s.io/apimachinery/pkg/watch"
	cache "k8s.io/client-go/tools/cache"
)

// VirtualMachineMACAddressLeaseInformer provides access to a shared informer and lister for
// VirtualMachineMACAddressLeases.
type VirtualMachineMACAddressLeaseInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() v1alpha2.VirtualMachineMACAddressLeaseLister
}

type virtualMachineMACAddressLeaseInformer struct {
	factory          internalinterfaces.SharedInformerFactory
	tweakListOptions internalinterfaces.TweakListOptionsFunc
}

// NewVirtualMachineMACAddressLeaseInformer constructs a new informer for VirtualMachineMACAddressLease type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewVirtualMachineMACAddressLeaseInformer(client versioned.Interface, resyncPeriod time.Duration, indexers cache.Indexers) cache.SharedIndexInformer {
	return NewFilteredVirtualMachineMACAddressLeaseInformer(client, resyncPeriod, indexers, nil)
}

// NewFilteredVirtualMachineMACAddressLeaseInformer constructs a new informer for VirtualMachineMACAddressLease type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewFilteredVirtualMachineMACAddressLeaseInformer(client versioned.Interface, resyncPeriod time.Duration, indexers cache.Indexers, tweakListOptions internalinterfaces.TweakListOptionsFunc) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options v1.ListOptions) (runtime.Object, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.VirtualizationV1alpha2().VirtualMachineMACAddressLeases().List(context.TODO(), options)
			},
			WatchFunc: func(options v1.ListOptions) (watch.Interface, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.VirtualizationV1alpha2().VirtualMachineMACAddressLeases().Watch(context.TODO(), options)
			},
		},
		&corev1alpha2.VirtualMachineMACAddressLease{},
		resyncPeriod,
		indexers,
	)
}

func (f *virtualMachineMACAddressLeaseInformer) defaultInformer(client versioned.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return NewFilteredVirtualMachineMACAddressLeaseInformer(client, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, f.tweakListOptions)
}

func (f *virtualMachineMACAddressLeaseInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&corev1alpha2.VirtualMachineMACAddressLease{}, f.defaultInformer)
}

func (f *virtualMachineMACAddressLeaseInformer) Lister() v1alpha2.VirtualMachineMACAddressLeaseLister {
	return v1alpha2.NewVirtualMachineMACAddressLeaseLister(f.Informer().GetIndexer())
}
