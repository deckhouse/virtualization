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

// FakeVirtualMachineCPUModels implements VirtualMachineCPUModelInterface
type FakeVirtualMachineCPUModels struct {
	Fake *FakeVirtualizationV1alpha2
	ns   string
}

var virtualmachinecpumodelsResource = v1alpha2.SchemeGroupVersion.WithResource("virtualmachinecpumodels")

var virtualmachinecpumodelsKind = v1alpha2.SchemeGroupVersion.WithKind("VirtualMachineCPUModel")

// Get takes name of the virtualMachineCPUModel, and returns the corresponding virtualMachineCPUModel object, and an error if there is any.
func (c *FakeVirtualMachineCPUModels) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1alpha2.VirtualMachineCPUModel, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(virtualmachinecpumodelsResource, c.ns, name), &v1alpha2.VirtualMachineCPUModel{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha2.VirtualMachineCPUModel), err
}

// List takes label and field selectors, and returns the list of VirtualMachineCPUModels that match those selectors.
func (c *FakeVirtualMachineCPUModels) List(ctx context.Context, opts v1.ListOptions) (result *v1alpha2.VirtualMachineCPUModelList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(virtualmachinecpumodelsResource, virtualmachinecpumodelsKind, c.ns, opts), &v1alpha2.VirtualMachineCPUModelList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha2.VirtualMachineCPUModelList{ListMeta: obj.(*v1alpha2.VirtualMachineCPUModelList).ListMeta}
	for _, item := range obj.(*v1alpha2.VirtualMachineCPUModelList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested virtualMachineCPUModels.
func (c *FakeVirtualMachineCPUModels) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(virtualmachinecpumodelsResource, c.ns, opts))

}

// Create takes the representation of a virtualMachineCPUModel and creates it.  Returns the server's representation of the virtualMachineCPUModel, and an error, if there is any.
func (c *FakeVirtualMachineCPUModels) Create(ctx context.Context, virtualMachineCPUModel *v1alpha2.VirtualMachineCPUModel, opts v1.CreateOptions) (result *v1alpha2.VirtualMachineCPUModel, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(virtualmachinecpumodelsResource, c.ns, virtualMachineCPUModel), &v1alpha2.VirtualMachineCPUModel{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha2.VirtualMachineCPUModel), err
}

// Update takes the representation of a virtualMachineCPUModel and updates it. Returns the server's representation of the virtualMachineCPUModel, and an error, if there is any.
func (c *FakeVirtualMachineCPUModels) Update(ctx context.Context, virtualMachineCPUModel *v1alpha2.VirtualMachineCPUModel, opts v1.UpdateOptions) (result *v1alpha2.VirtualMachineCPUModel, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(virtualmachinecpumodelsResource, c.ns, virtualMachineCPUModel), &v1alpha2.VirtualMachineCPUModel{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha2.VirtualMachineCPUModel), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeVirtualMachineCPUModels) UpdateStatus(ctx context.Context, virtualMachineCPUModel *v1alpha2.VirtualMachineCPUModel, opts v1.UpdateOptions) (*v1alpha2.VirtualMachineCPUModel, error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceAction(virtualmachinecpumodelsResource, "status", c.ns, virtualMachineCPUModel), &v1alpha2.VirtualMachineCPUModel{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha2.VirtualMachineCPUModel), err
}

// Delete takes name of the virtualMachineCPUModel and deletes it. Returns an error if one occurs.
func (c *FakeVirtualMachineCPUModels) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteActionWithOptions(virtualmachinecpumodelsResource, c.ns, name, opts), &v1alpha2.VirtualMachineCPUModel{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeVirtualMachineCPUModels) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(virtualmachinecpumodelsResource, c.ns, listOpts)

	_, err := c.Fake.Invokes(action, &v1alpha2.VirtualMachineCPUModelList{})
	return err
}

// Patch applies the patch and returns the patched virtualMachineCPUModel.
func (c *FakeVirtualMachineCPUModels) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha2.VirtualMachineCPUModel, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(virtualmachinecpumodelsResource, c.ns, name, pt, data, subresources...), &v1alpha2.VirtualMachineCPUModel{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha2.VirtualMachineCPUModel), err
}
