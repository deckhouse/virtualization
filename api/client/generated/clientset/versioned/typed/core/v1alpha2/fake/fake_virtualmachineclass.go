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

// FakeVirtualMachineClasses implements VirtualMachineClassInterface
type FakeVirtualMachineClasses struct {
	Fake *FakeVirtualizationV1alpha2
}

var virtualmachineclassesResource = v1alpha2.SchemeGroupVersion.WithResource("virtualmachineclasses")

var virtualmachineclassesKind = v1alpha2.SchemeGroupVersion.WithKind("VirtualMachineClass")

// Get takes name of the virtualMachineClass, and returns the corresponding virtualMachineClass object, and an error if there is any.
func (c *FakeVirtualMachineClasses) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1alpha2.VirtualMachineClass, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootGetAction(virtualmachineclassesResource, name), &v1alpha2.VirtualMachineClass{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha2.VirtualMachineClass), err
}

// List takes label and field selectors, and returns the list of VirtualMachineClasses that match those selectors.
func (c *FakeVirtualMachineClasses) List(ctx context.Context, opts v1.ListOptions) (result *v1alpha2.VirtualMachineClassList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootListAction(virtualmachineclassesResource, virtualmachineclassesKind, opts), &v1alpha2.VirtualMachineClassList{})
	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha2.VirtualMachineClassList{ListMeta: obj.(*v1alpha2.VirtualMachineClassList).ListMeta}
	for _, item := range obj.(*v1alpha2.VirtualMachineClassList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested virtualMachineClasses.
func (c *FakeVirtualMachineClasses) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewRootWatchAction(virtualmachineclassesResource, opts))
}

// Create takes the representation of a virtualMachineClass and creates it.  Returns the server's representation of the virtualMachineClass, and an error, if there is any.
func (c *FakeVirtualMachineClasses) Create(ctx context.Context, virtualMachineClass *v1alpha2.VirtualMachineClass, opts v1.CreateOptions) (result *v1alpha2.VirtualMachineClass, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootCreateAction(virtualmachineclassesResource, virtualMachineClass), &v1alpha2.VirtualMachineClass{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha2.VirtualMachineClass), err
}

// Update takes the representation of a virtualMachineClass and updates it. Returns the server's representation of the virtualMachineClass, and an error, if there is any.
func (c *FakeVirtualMachineClasses) Update(ctx context.Context, virtualMachineClass *v1alpha2.VirtualMachineClass, opts v1.UpdateOptions) (result *v1alpha2.VirtualMachineClass, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootUpdateAction(virtualmachineclassesResource, virtualMachineClass), &v1alpha2.VirtualMachineClass{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha2.VirtualMachineClass), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeVirtualMachineClasses) UpdateStatus(ctx context.Context, virtualMachineClass *v1alpha2.VirtualMachineClass, opts v1.UpdateOptions) (*v1alpha2.VirtualMachineClass, error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootUpdateSubresourceAction(virtualmachineclassesResource, "status", virtualMachineClass), &v1alpha2.VirtualMachineClass{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha2.VirtualMachineClass), err
}

// Delete takes name of the virtualMachineClass and deletes it. Returns an error if one occurs.
func (c *FakeVirtualMachineClasses) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewRootDeleteActionWithOptions(virtualmachineclassesResource, name, opts), &v1alpha2.VirtualMachineClass{})
	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeVirtualMachineClasses) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewRootDeleteCollectionAction(virtualmachineclassesResource, listOpts)

	_, err := c.Fake.Invokes(action, &v1alpha2.VirtualMachineClassList{})
	return err
}

// Patch applies the patch and returns the patched virtualMachineClass.
func (c *FakeVirtualMachineClasses) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha2.VirtualMachineClass, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootPatchSubresourceAction(virtualmachineclassesResource, name, pt, data, subresources...), &v1alpha2.VirtualMachineClass{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha2.VirtualMachineClass), err
}
