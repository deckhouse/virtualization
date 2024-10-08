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

// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	"context"

	v1alpha2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeVirtualMachineIPAddresses implements VirtualMachineIPAddressInterface
type FakeVirtualMachineIPAddresses struct {
	Fake *FakeVirtualizationV1alpha2
	ns   string
}

var virtualmachineipaddressesResource = v1alpha2.SchemeGroupVersion.WithResource("virtualmachineipaddresses")

var virtualmachineipaddressesKind = v1alpha2.SchemeGroupVersion.WithKind("VirtualMachineIPAddress")

// Get takes name of the virtualMachineIPAddress, and returns the corresponding virtualMachineIPAddress object, and an error if there is any.
func (c *FakeVirtualMachineIPAddresses) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1alpha2.VirtualMachineIPAddress, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(virtualmachineipaddressesResource, c.ns, name), &v1alpha2.VirtualMachineIPAddress{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha2.VirtualMachineIPAddress), err
}

// List takes label and field selectors, and returns the list of VirtualMachineIPAddresses that match those selectors.
func (c *FakeVirtualMachineIPAddresses) List(ctx context.Context, opts v1.ListOptions) (result *v1alpha2.VirtualMachineIPAddressList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(virtualmachineipaddressesResource, virtualmachineipaddressesKind, c.ns, opts), &v1alpha2.VirtualMachineIPAddressList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha2.VirtualMachineIPAddressList{ListMeta: obj.(*v1alpha2.VirtualMachineIPAddressList).ListMeta}
	for _, item := range obj.(*v1alpha2.VirtualMachineIPAddressList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested virtualMachineIPAddresses.
func (c *FakeVirtualMachineIPAddresses) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(virtualmachineipaddressesResource, c.ns, opts))

}

// Create takes the representation of a virtualMachineIPAddress and creates it.  Returns the server's representation of the virtualMachineIPAddress, and an error, if there is any.
func (c *FakeVirtualMachineIPAddresses) Create(ctx context.Context, virtualMachineIPAddress *v1alpha2.VirtualMachineIPAddress, opts v1.CreateOptions) (result *v1alpha2.VirtualMachineIPAddress, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(virtualmachineipaddressesResource, c.ns, virtualMachineIPAddress), &v1alpha2.VirtualMachineIPAddress{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha2.VirtualMachineIPAddress), err
}

// Update takes the representation of a virtualMachineIPAddress and updates it. Returns the server's representation of the virtualMachineIPAddress, and an error, if there is any.
func (c *FakeVirtualMachineIPAddresses) Update(ctx context.Context, virtualMachineIPAddress *v1alpha2.VirtualMachineIPAddress, opts v1.UpdateOptions) (result *v1alpha2.VirtualMachineIPAddress, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(virtualmachineipaddressesResource, c.ns, virtualMachineIPAddress), &v1alpha2.VirtualMachineIPAddress{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha2.VirtualMachineIPAddress), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeVirtualMachineIPAddresses) UpdateStatus(ctx context.Context, virtualMachineIPAddress *v1alpha2.VirtualMachineIPAddress, opts v1.UpdateOptions) (*v1alpha2.VirtualMachineIPAddress, error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceAction(virtualmachineipaddressesResource, "status", c.ns, virtualMachineIPAddress), &v1alpha2.VirtualMachineIPAddress{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha2.VirtualMachineIPAddress), err
}

// Delete takes name of the virtualMachineIPAddress and deletes it. Returns an error if one occurs.
func (c *FakeVirtualMachineIPAddresses) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteActionWithOptions(virtualmachineipaddressesResource, c.ns, name, opts), &v1alpha2.VirtualMachineIPAddress{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeVirtualMachineIPAddresses) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(virtualmachineipaddressesResource, c.ns, listOpts)

	_, err := c.Fake.Invokes(action, &v1alpha2.VirtualMachineIPAddressList{})
	return err
}

// Patch applies the patch and returns the patched virtualMachineIPAddress.
func (c *FakeVirtualMachineIPAddresses) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha2.VirtualMachineIPAddress, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(virtualmachineipaddressesResource, c.ns, name, pt, data, subresources...), &v1alpha2.VirtualMachineIPAddress{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha2.VirtualMachineIPAddress), err
}
