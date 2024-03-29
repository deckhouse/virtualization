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

	v1alpha2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeVirtualMachineIPAddressClaims implements VirtualMachineIPAddressClaimInterface
type FakeVirtualMachineIPAddressClaims struct {
	Fake *FakeVirtualizationV1alpha2
	ns   string
}

var virtualmachineipaddressclaimsResource = v1alpha2.SchemeGroupVersion.WithResource("virtualmachineipaddressclaims")

var virtualmachineipaddressclaimsKind = v1alpha2.SchemeGroupVersion.WithKind("VirtualMachineIPAddressClaim")

// Get takes name of the virtualMachineIPAddressClaim, and returns the corresponding virtualMachineIPAddressClaim object, and an error if there is any.
func (c *FakeVirtualMachineIPAddressClaims) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1alpha2.VirtualMachineIPAddressClaim, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(virtualmachineipaddressclaimsResource, c.ns, name), &v1alpha2.VirtualMachineIPAddressClaim{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha2.VirtualMachineIPAddressClaim), err
}

// List takes label and field selectors, and returns the list of VirtualMachineIPAddressClaims that match those selectors.
func (c *FakeVirtualMachineIPAddressClaims) List(ctx context.Context, opts v1.ListOptions) (result *v1alpha2.VirtualMachineIPAddressClaimList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(virtualmachineipaddressclaimsResource, virtualmachineipaddressclaimsKind, c.ns, opts), &v1alpha2.VirtualMachineIPAddressClaimList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha2.VirtualMachineIPAddressClaimList{ListMeta: obj.(*v1alpha2.VirtualMachineIPAddressClaimList).ListMeta}
	for _, item := range obj.(*v1alpha2.VirtualMachineIPAddressClaimList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested virtualMachineIPAddressClaims.
func (c *FakeVirtualMachineIPAddressClaims) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(virtualmachineipaddressclaimsResource, c.ns, opts))

}

// Create takes the representation of a virtualMachineIPAddressClaim and creates it.  Returns the server's representation of the virtualMachineIPAddressClaim, and an error, if there is any.
func (c *FakeVirtualMachineIPAddressClaims) Create(ctx context.Context, virtualMachineIPAddressClaim *v1alpha2.VirtualMachineIPAddressClaim, opts v1.CreateOptions) (result *v1alpha2.VirtualMachineIPAddressClaim, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(virtualmachineipaddressclaimsResource, c.ns, virtualMachineIPAddressClaim), &v1alpha2.VirtualMachineIPAddressClaim{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha2.VirtualMachineIPAddressClaim), err
}

// Update takes the representation of a virtualMachineIPAddressClaim and updates it. Returns the server's representation of the virtualMachineIPAddressClaim, and an error, if there is any.
func (c *FakeVirtualMachineIPAddressClaims) Update(ctx context.Context, virtualMachineIPAddressClaim *v1alpha2.VirtualMachineIPAddressClaim, opts v1.UpdateOptions) (result *v1alpha2.VirtualMachineIPAddressClaim, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(virtualmachineipaddressclaimsResource, c.ns, virtualMachineIPAddressClaim), &v1alpha2.VirtualMachineIPAddressClaim{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha2.VirtualMachineIPAddressClaim), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeVirtualMachineIPAddressClaims) UpdateStatus(ctx context.Context, virtualMachineIPAddressClaim *v1alpha2.VirtualMachineIPAddressClaim, opts v1.UpdateOptions) (*v1alpha2.VirtualMachineIPAddressClaim, error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceAction(virtualmachineipaddressclaimsResource, "status", c.ns, virtualMachineIPAddressClaim), &v1alpha2.VirtualMachineIPAddressClaim{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha2.VirtualMachineIPAddressClaim), err
}

// Delete takes name of the virtualMachineIPAddressClaim and deletes it. Returns an error if one occurs.
func (c *FakeVirtualMachineIPAddressClaims) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteActionWithOptions(virtualmachineipaddressclaimsResource, c.ns, name, opts), &v1alpha2.VirtualMachineIPAddressClaim{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeVirtualMachineIPAddressClaims) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(virtualmachineipaddressclaimsResource, c.ns, listOpts)

	_, err := c.Fake.Invokes(action, &v1alpha2.VirtualMachineIPAddressClaimList{})
	return err
}

// Patch applies the patch and returns the patched virtualMachineIPAddressClaim.
func (c *FakeVirtualMachineIPAddressClaims) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha2.VirtualMachineIPAddressClaim, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(virtualmachineipaddressclaimsResource, c.ns, name, pt, data, subresources...), &v1alpha2.VirtualMachineIPAddressClaim{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha2.VirtualMachineIPAddressClaim), err
}
