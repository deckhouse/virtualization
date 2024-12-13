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

// Code generated by lister-gen. DO NOT EDIT.

package v1alpha2

import (
	v1alpha2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// VirtualMachineMACAddressLister helps list VirtualMachineMACAddresses.
// All objects returned here must be treated as read-only.
type VirtualMachineMACAddressLister interface {
	// List lists all VirtualMachineMACAddresses in the indexer.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha2.VirtualMachineMACAddress, err error)
	// VirtualMachineMACAddresses returns an object that can list and get VirtualMachineMACAddresses.
	VirtualMachineMACAddresses(namespace string) VirtualMachineMACAddressNamespaceLister
	VirtualMachineMACAddressListerExpansion
}

// virtualMachineMACAddressLister implements the VirtualMachineMACAddressLister interface.
type virtualMachineMACAddressLister struct {
	indexer cache.Indexer
}

// NewVirtualMachineMACAddressLister returns a new VirtualMachineMACAddressLister.
func NewVirtualMachineMACAddressLister(indexer cache.Indexer) VirtualMachineMACAddressLister {
	return &virtualMachineMACAddressLister{indexer: indexer}
}

// List lists all VirtualMachineMACAddresses in the indexer.
func (s *virtualMachineMACAddressLister) List(selector labels.Selector) (ret []*v1alpha2.VirtualMachineMACAddress, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha2.VirtualMachineMACAddress))
	})
	return ret, err
}

// VirtualMachineMACAddresses returns an object that can list and get VirtualMachineMACAddresses.
func (s *virtualMachineMACAddressLister) VirtualMachineMACAddresses(namespace string) VirtualMachineMACAddressNamespaceLister {
	return virtualMachineMACAddressNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// VirtualMachineMACAddressNamespaceLister helps list and get VirtualMachineMACAddresses.
// All objects returned here must be treated as read-only.
type VirtualMachineMACAddressNamespaceLister interface {
	// List lists all VirtualMachineMACAddresses in the indexer for a given namespace.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha2.VirtualMachineMACAddress, err error)
	// Get retrieves the VirtualMachineMACAddress from the indexer for a given namespace and name.
	// Objects returned here must be treated as read-only.
	Get(name string) (*v1alpha2.VirtualMachineMACAddress, error)
	VirtualMachineMACAddressNamespaceListerExpansion
}

// virtualMachineMACAddressNamespaceLister implements the VirtualMachineMACAddressNamespaceLister
// interface.
type virtualMachineMACAddressNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all VirtualMachineMACAddresses in the indexer for a given namespace.
func (s virtualMachineMACAddressNamespaceLister) List(selector labels.Selector) (ret []*v1alpha2.VirtualMachineMACAddress, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha2.VirtualMachineMACAddress))
	})
	return ret, err
}

// Get retrieves the VirtualMachineMACAddress from the indexer for a given namespace and name.
func (s virtualMachineMACAddressNamespaceLister) Get(name string) (*v1alpha2.VirtualMachineMACAddress, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1alpha2.Resource("virtualmachinemacaddress"), name)
	}
	return obj.(*v1alpha2.VirtualMachineMACAddress), nil
}
