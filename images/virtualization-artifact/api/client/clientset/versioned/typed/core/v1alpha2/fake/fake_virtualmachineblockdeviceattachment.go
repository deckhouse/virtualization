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

// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	"context"

	v1alpha2 "github.com/deckhouse/virtualization-controller/api/core/v1alpha2"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeVirtualMachineBlockDeviceAttachments implements VirtualMachineBlockDeviceAttachmentInterface
type FakeVirtualMachineBlockDeviceAttachments struct {
	Fake *FakeVirtualizationV1alpha2
	ns   string
}

var virtualmachineblockdeviceattachmentsResource = v1alpha2.SchemeGroupVersion.WithResource("virtualmachineblockdeviceattachments")

var virtualmachineblockdeviceattachmentsKind = v1alpha2.SchemeGroupVersion.WithKind("VirtualMachineBlockDeviceAttachment")

// Get takes name of the virtualMachineBlockDeviceAttachment, and returns the corresponding virtualMachineBlockDeviceAttachment object, and an error if there is any.
func (c *FakeVirtualMachineBlockDeviceAttachments) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1alpha2.VirtualMachineBlockDeviceAttachment, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(virtualmachineblockdeviceattachmentsResource, c.ns, name), &v1alpha2.VirtualMachineBlockDeviceAttachment{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha2.VirtualMachineBlockDeviceAttachment), err
}

// List takes label and field selectors, and returns the list of VirtualMachineBlockDeviceAttachments that match those selectors.
func (c *FakeVirtualMachineBlockDeviceAttachments) List(ctx context.Context, opts v1.ListOptions) (result *v1alpha2.VirtualMachineBlockDeviceAttachmentList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(virtualmachineblockdeviceattachmentsResource, virtualmachineblockdeviceattachmentsKind, c.ns, opts), &v1alpha2.VirtualMachineBlockDeviceAttachmentList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha2.VirtualMachineBlockDeviceAttachmentList{ListMeta: obj.(*v1alpha2.VirtualMachineBlockDeviceAttachmentList).ListMeta}
	for _, item := range obj.(*v1alpha2.VirtualMachineBlockDeviceAttachmentList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested virtualMachineBlockDeviceAttachments.
func (c *FakeVirtualMachineBlockDeviceAttachments) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(virtualmachineblockdeviceattachmentsResource, c.ns, opts))

}

// Create takes the representation of a virtualMachineBlockDeviceAttachment and creates it.  Returns the server's representation of the virtualMachineBlockDeviceAttachment, and an error, if there is any.
func (c *FakeVirtualMachineBlockDeviceAttachments) Create(ctx context.Context, virtualMachineBlockDeviceAttachment *v1alpha2.VirtualMachineBlockDeviceAttachment, opts v1.CreateOptions) (result *v1alpha2.VirtualMachineBlockDeviceAttachment, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(virtualmachineblockdeviceattachmentsResource, c.ns, virtualMachineBlockDeviceAttachment), &v1alpha2.VirtualMachineBlockDeviceAttachment{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha2.VirtualMachineBlockDeviceAttachment), err
}

// Update takes the representation of a virtualMachineBlockDeviceAttachment and updates it. Returns the server's representation of the virtualMachineBlockDeviceAttachment, and an error, if there is any.
func (c *FakeVirtualMachineBlockDeviceAttachments) Update(ctx context.Context, virtualMachineBlockDeviceAttachment *v1alpha2.VirtualMachineBlockDeviceAttachment, opts v1.UpdateOptions) (result *v1alpha2.VirtualMachineBlockDeviceAttachment, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(virtualmachineblockdeviceattachmentsResource, c.ns, virtualMachineBlockDeviceAttachment), &v1alpha2.VirtualMachineBlockDeviceAttachment{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha2.VirtualMachineBlockDeviceAttachment), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeVirtualMachineBlockDeviceAttachments) UpdateStatus(ctx context.Context, virtualMachineBlockDeviceAttachment *v1alpha2.VirtualMachineBlockDeviceAttachment, opts v1.UpdateOptions) (*v1alpha2.VirtualMachineBlockDeviceAttachment, error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceAction(virtualmachineblockdeviceattachmentsResource, "status", c.ns, virtualMachineBlockDeviceAttachment), &v1alpha2.VirtualMachineBlockDeviceAttachment{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha2.VirtualMachineBlockDeviceAttachment), err
}

// Delete takes name of the virtualMachineBlockDeviceAttachment and deletes it. Returns an error if one occurs.
func (c *FakeVirtualMachineBlockDeviceAttachments) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteActionWithOptions(virtualmachineblockdeviceattachmentsResource, c.ns, name, opts), &v1alpha2.VirtualMachineBlockDeviceAttachment{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeVirtualMachineBlockDeviceAttachments) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(virtualmachineblockdeviceattachmentsResource, c.ns, listOpts)

	_, err := c.Fake.Invokes(action, &v1alpha2.VirtualMachineBlockDeviceAttachmentList{})
	return err
}

// Patch applies the patch and returns the patched virtualMachineBlockDeviceAttachment.
func (c *FakeVirtualMachineBlockDeviceAttachments) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha2.VirtualMachineBlockDeviceAttachment, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(virtualmachineblockdeviceattachmentsResource, c.ns, name, pt, data, subresources...), &v1alpha2.VirtualMachineBlockDeviceAttachment{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha2.VirtualMachineBlockDeviceAttachment), err
}
