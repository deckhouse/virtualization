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

// FakeVirtualDisks implements VirtualDiskInterface
type FakeVirtualDisks struct {
	Fake *FakeVirtualizationV1alpha2
	ns   string
}

var virtualdisksResource = v1alpha2.SchemeGroupVersion.WithResource("virtualdisks")

var virtualdisksKind = v1alpha2.SchemeGroupVersion.WithKind("VirtualDisk")

// Get takes name of the virtualDisk, and returns the corresponding virtualDisk object, and an error if there is any.
func (c *FakeVirtualDisks) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1alpha2.VirtualDisk, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(virtualdisksResource, c.ns, name), &v1alpha2.VirtualDisk{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha2.VirtualDisk), err
}

// List takes label and field selectors, and returns the list of VirtualDisks that match those selectors.
func (c *FakeVirtualDisks) List(ctx context.Context, opts v1.ListOptions) (result *v1alpha2.VirtualDiskList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(virtualdisksResource, virtualdisksKind, c.ns, opts), &v1alpha2.VirtualDiskList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha2.VirtualDiskList{ListMeta: obj.(*v1alpha2.VirtualDiskList).ListMeta}
	for _, item := range obj.(*v1alpha2.VirtualDiskList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested virtualDisks.
func (c *FakeVirtualDisks) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(virtualdisksResource, c.ns, opts))

}

// Create takes the representation of a virtualDisk and creates it.  Returns the server's representation of the virtualDisk, and an error, if there is any.
func (c *FakeVirtualDisks) Create(ctx context.Context, virtualDisk *v1alpha2.VirtualDisk, opts v1.CreateOptions) (result *v1alpha2.VirtualDisk, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(virtualdisksResource, c.ns, virtualDisk), &v1alpha2.VirtualDisk{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha2.VirtualDisk), err
}

// Update takes the representation of a virtualDisk and updates it. Returns the server's representation of the virtualDisk, and an error, if there is any.
func (c *FakeVirtualDisks) Update(ctx context.Context, virtualDisk *v1alpha2.VirtualDisk, opts v1.UpdateOptions) (result *v1alpha2.VirtualDisk, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(virtualdisksResource, c.ns, virtualDisk), &v1alpha2.VirtualDisk{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha2.VirtualDisk), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeVirtualDisks) UpdateStatus(ctx context.Context, virtualDisk *v1alpha2.VirtualDisk, opts v1.UpdateOptions) (*v1alpha2.VirtualDisk, error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceAction(virtualdisksResource, "status", c.ns, virtualDisk), &v1alpha2.VirtualDisk{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha2.VirtualDisk), err
}

// Delete takes name of the virtualDisk and deletes it. Returns an error if one occurs.
func (c *FakeVirtualDisks) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteActionWithOptions(virtualdisksResource, c.ns, name, opts), &v1alpha2.VirtualDisk{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeVirtualDisks) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(virtualdisksResource, c.ns, listOpts)

	_, err := c.Fake.Invokes(action, &v1alpha2.VirtualDiskList{})
	return err
}

// Patch applies the patch and returns the patched virtualDisk.
func (c *FakeVirtualDisks) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha2.VirtualDisk, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(virtualdisksResource, c.ns, name, pt, data, subresources...), &v1alpha2.VirtualDisk{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha2.VirtualDisk), err
}
