/*
Copyright 2022 Flant JSC

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

// VirtualMachineIPAddressClaimLister helps list VirtualMachineIPAddressClaims.
// All objects returned here must be treated as read-only.
type VirtualMachineIPAddressClaimLister interface {
	// List lists all VirtualMachineIPAddressClaims in the indexer.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha2.VirtualMachineIPAddressClaim, err error)
	// VirtualMachineIPAddressClaims returns an object that can list and get VirtualMachineIPAddressClaims.
	VirtualMachineIPAddressClaims(namespace string) VirtualMachineIPAddressClaimNamespaceLister
	VirtualMachineIPAddressClaimListerExpansion
}

// virtualMachineIPAddressClaimLister implements the VirtualMachineIPAddressClaimLister interface.
type virtualMachineIPAddressClaimLister struct {
	indexer cache.Indexer
}

// NewVirtualMachineIPAddressClaimLister returns a new VirtualMachineIPAddressClaimLister.
func NewVirtualMachineIPAddressClaimLister(indexer cache.Indexer) VirtualMachineIPAddressClaimLister {
	return &virtualMachineIPAddressClaimLister{indexer: indexer}
}

// List lists all VirtualMachineIPAddressClaims in the indexer.
func (s *virtualMachineIPAddressClaimLister) List(selector labels.Selector) (ret []*v1alpha2.VirtualMachineIPAddressClaim, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha2.VirtualMachineIPAddressClaim))
	})
	return ret, err
}

// VirtualMachineIPAddressClaims returns an object that can list and get VirtualMachineIPAddressClaims.
func (s *virtualMachineIPAddressClaimLister) VirtualMachineIPAddressClaims(namespace string) VirtualMachineIPAddressClaimNamespaceLister {
	return virtualMachineIPAddressClaimNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// VirtualMachineIPAddressClaimNamespaceLister helps list and get VirtualMachineIPAddressClaims.
// All objects returned here must be treated as read-only.
type VirtualMachineIPAddressClaimNamespaceLister interface {
	// List lists all VirtualMachineIPAddressClaims in the indexer for a given namespace.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha2.VirtualMachineIPAddressClaim, err error)
	// Get retrieves the VirtualMachineIPAddressClaim from the indexer for a given namespace and name.
	// Objects returned here must be treated as read-only.
	Get(name string) (*v1alpha2.VirtualMachineIPAddressClaim, error)
	VirtualMachineIPAddressClaimNamespaceListerExpansion
}

// virtualMachineIPAddressClaimNamespaceLister implements the VirtualMachineIPAddressClaimNamespaceLister
// interface.
type virtualMachineIPAddressClaimNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all VirtualMachineIPAddressClaims in the indexer for a given namespace.
func (s virtualMachineIPAddressClaimNamespaceLister) List(selector labels.Selector) (ret []*v1alpha2.VirtualMachineIPAddressClaim, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha2.VirtualMachineIPAddressClaim))
	})
	return ret, err
}

// Get retrieves the VirtualMachineIPAddressClaim from the indexer for a given namespace and name.
func (s virtualMachineIPAddressClaimNamespaceLister) Get(name string) (*v1alpha2.VirtualMachineIPAddressClaim, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1alpha2.Resource("virtualmachineipaddressclaim"), name)
	}
	return obj.(*v1alpha2.VirtualMachineIPAddressClaim), nil
}
