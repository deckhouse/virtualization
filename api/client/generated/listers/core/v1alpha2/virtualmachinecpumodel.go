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
	v1alpha2 "github.com/deckhouse/virtualization-controller/api/core/v1alpha2"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// VirtualMachineCPUModelLister helps list VirtualMachineCPUModels.
// All objects returned here must be treated as read-only.
type VirtualMachineCPUModelLister interface {
	// List lists all VirtualMachineCPUModels in the indexer.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha2.VirtualMachineCPUModel, err error)
	// VirtualMachineCPUModels returns an object that can list and get VirtualMachineCPUModels.
	VirtualMachineCPUModels(namespace string) VirtualMachineCPUModelNamespaceLister
	VirtualMachineCPUModelListerExpansion
}

// virtualMachineCPUModelLister implements the VirtualMachineCPUModelLister interface.
type virtualMachineCPUModelLister struct {
	indexer cache.Indexer
}

// NewVirtualMachineCPUModelLister returns a new VirtualMachineCPUModelLister.
func NewVirtualMachineCPUModelLister(indexer cache.Indexer) VirtualMachineCPUModelLister {
	return &virtualMachineCPUModelLister{indexer: indexer}
}

// List lists all VirtualMachineCPUModels in the indexer.
func (s *virtualMachineCPUModelLister) List(selector labels.Selector) (ret []*v1alpha2.VirtualMachineCPUModel, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha2.VirtualMachineCPUModel))
	})
	return ret, err
}

// VirtualMachineCPUModels returns an object that can list and get VirtualMachineCPUModels.
func (s *virtualMachineCPUModelLister) VirtualMachineCPUModels(namespace string) VirtualMachineCPUModelNamespaceLister {
	return virtualMachineCPUModelNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// VirtualMachineCPUModelNamespaceLister helps list and get VirtualMachineCPUModels.
// All objects returned here must be treated as read-only.
type VirtualMachineCPUModelNamespaceLister interface {
	// List lists all VirtualMachineCPUModels in the indexer for a given namespace.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha2.VirtualMachineCPUModel, err error)
	// Get retrieves the VirtualMachineCPUModel from the indexer for a given namespace and name.
	// Objects returned here must be treated as read-only.
	Get(name string) (*v1alpha2.VirtualMachineCPUModel, error)
	VirtualMachineCPUModelNamespaceListerExpansion
}

// virtualMachineCPUModelNamespaceLister implements the VirtualMachineCPUModelNamespaceLister
// interface.
type virtualMachineCPUModelNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all VirtualMachineCPUModels in the indexer for a given namespace.
func (s virtualMachineCPUModelNamespaceLister) List(selector labels.Selector) (ret []*v1alpha2.VirtualMachineCPUModel, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha2.VirtualMachineCPUModel))
	})
	return ret, err
}

// Get retrieves the VirtualMachineCPUModel from the indexer for a given namespace and name.
func (s virtualMachineCPUModelNamespaceLister) Get(name string) (*v1alpha2.VirtualMachineCPUModel, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1alpha2.Resource("virtualmachinecpumodel"), name)
	}
	return obj.(*v1alpha2.VirtualMachineCPUModel), nil
}
