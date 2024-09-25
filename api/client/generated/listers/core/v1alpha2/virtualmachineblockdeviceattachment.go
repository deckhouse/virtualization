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

// VirtualMachineBlockDeviceAttachmentLister helps list VirtualMachineBlockDeviceAttachments.
// All objects returned here must be treated as read-only.
type VirtualMachineBlockDeviceAttachmentLister interface {
	// List lists all VirtualMachineBlockDeviceAttachments in the indexer.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha2.VirtualMachineBlockDeviceAttachment, err error)
	// VirtualMachineBlockDeviceAttachments returns an object that can list and get VirtualMachineBlockDeviceAttachments.
	VirtualMachineBlockDeviceAttachments(namespace string) VirtualMachineBlockDeviceAttachmentNamespaceLister
	VirtualMachineBlockDeviceAttachmentListerExpansion
}

// virtualMachineBlockDeviceAttachmentLister implements the VirtualMachineBlockDeviceAttachmentLister interface.
type virtualMachineBlockDeviceAttachmentLister struct {
	indexer cache.Indexer
}

// NewVirtualMachineBlockDeviceAttachmentLister returns a new VirtualMachineBlockDeviceAttachmentLister.
func NewVirtualMachineBlockDeviceAttachmentLister(indexer cache.Indexer) VirtualMachineBlockDeviceAttachmentLister {
	return &virtualMachineBlockDeviceAttachmentLister{indexer: indexer}
}

// List lists all VirtualMachineBlockDeviceAttachments in the indexer.
func (s *virtualMachineBlockDeviceAttachmentLister) List(selector labels.Selector) (ret []*v1alpha2.VirtualMachineBlockDeviceAttachment, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha2.VirtualMachineBlockDeviceAttachment))
	})
	return ret, err
}

// VirtualMachineBlockDeviceAttachments returns an object that can list and get VirtualMachineBlockDeviceAttachments.
func (s *virtualMachineBlockDeviceAttachmentLister) VirtualMachineBlockDeviceAttachments(namespace string) VirtualMachineBlockDeviceAttachmentNamespaceLister {
	return virtualMachineBlockDeviceAttachmentNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// VirtualMachineBlockDeviceAttachmentNamespaceLister helps list and get VirtualMachineBlockDeviceAttachments.
// All objects returned here must be treated as read-only.
type VirtualMachineBlockDeviceAttachmentNamespaceLister interface {
	// List lists all VirtualMachineBlockDeviceAttachments in the indexer for a given namespace.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha2.VirtualMachineBlockDeviceAttachment, err error)
	// Get retrieves the VirtualMachineBlockDeviceAttachment from the indexer for a given namespace and name.
	// Objects returned here must be treated as read-only.
	Get(name string) (*v1alpha2.VirtualMachineBlockDeviceAttachment, error)
	VirtualMachineBlockDeviceAttachmentNamespaceListerExpansion
}

// virtualMachineBlockDeviceAttachmentNamespaceLister implements the VirtualMachineBlockDeviceAttachmentNamespaceLister
// interface.
type virtualMachineBlockDeviceAttachmentNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all VirtualMachineBlockDeviceAttachments in the indexer for a given namespace.
func (s virtualMachineBlockDeviceAttachmentNamespaceLister) List(selector labels.Selector) (ret []*v1alpha2.VirtualMachineBlockDeviceAttachment, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha2.VirtualMachineBlockDeviceAttachment))
	})
	return ret, err
}

// Get retrieves the VirtualMachineBlockDeviceAttachment from the indexer for a given namespace and name.
func (s virtualMachineBlockDeviceAttachmentNamespaceLister) Get(name string) (*v1alpha2.VirtualMachineBlockDeviceAttachment, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1alpha2.Resource("virtualmachineblockdeviceattachment"), name)
	}
	return obj.(*v1alpha2.VirtualMachineBlockDeviceAttachment), nil
}
