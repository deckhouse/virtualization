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

// FakeVirtualMachineSnapshots implements VirtualMachineSnapshotInterface
type FakeVirtualMachineSnapshots struct {
	Fake *FakeVirtualizationV1alpha2
	ns   string
}

var virtualmachinesnapshotsResource = v1alpha2.SchemeGroupVersion.WithResource("virtualmachinesnapshots")

var virtualmachinesnapshotsKind = v1alpha2.SchemeGroupVersion.WithKind("VirtualMachineSnapshot")

// Get takes name of the virtualMachineSnapshot, and returns the corresponding virtualMachineSnapshot object, and an error if there is any.
func (c *FakeVirtualMachineSnapshots) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1alpha2.VirtualMachineSnapshot, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(virtualmachinesnapshotsResource, c.ns, name), &v1alpha2.VirtualMachineSnapshot{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha2.VirtualMachineSnapshot), err
}

// List takes label and field selectors, and returns the list of VirtualMachineSnapshots that match those selectors.
func (c *FakeVirtualMachineSnapshots) List(ctx context.Context, opts v1.ListOptions) (result *v1alpha2.VirtualMachineSnapshotList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(virtualmachinesnapshotsResource, virtualmachinesnapshotsKind, c.ns, opts), &v1alpha2.VirtualMachineSnapshotList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha2.VirtualMachineSnapshotList{ListMeta: obj.(*v1alpha2.VirtualMachineSnapshotList).ListMeta}
	for _, item := range obj.(*v1alpha2.VirtualMachineSnapshotList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested virtualMachineSnapshots.
func (c *FakeVirtualMachineSnapshots) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(virtualmachinesnapshotsResource, c.ns, opts))

}

// Create takes the representation of a virtualMachineSnapshot and creates it.  Returns the server's representation of the virtualMachineSnapshot, and an error, if there is any.
func (c *FakeVirtualMachineSnapshots) Create(ctx context.Context, virtualMachineSnapshot *v1alpha2.VirtualMachineSnapshot, opts v1.CreateOptions) (result *v1alpha2.VirtualMachineSnapshot, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(virtualmachinesnapshotsResource, c.ns, virtualMachineSnapshot), &v1alpha2.VirtualMachineSnapshot{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha2.VirtualMachineSnapshot), err
}

// Update takes the representation of a virtualMachineSnapshot and updates it. Returns the server's representation of the virtualMachineSnapshot, and an error, if there is any.
func (c *FakeVirtualMachineSnapshots) Update(ctx context.Context, virtualMachineSnapshot *v1alpha2.VirtualMachineSnapshot, opts v1.UpdateOptions) (result *v1alpha2.VirtualMachineSnapshot, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(virtualmachinesnapshotsResource, c.ns, virtualMachineSnapshot), &v1alpha2.VirtualMachineSnapshot{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha2.VirtualMachineSnapshot), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeVirtualMachineSnapshots) UpdateStatus(ctx context.Context, virtualMachineSnapshot *v1alpha2.VirtualMachineSnapshot, opts v1.UpdateOptions) (*v1alpha2.VirtualMachineSnapshot, error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceAction(virtualmachinesnapshotsResource, "status", c.ns, virtualMachineSnapshot), &v1alpha2.VirtualMachineSnapshot{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha2.VirtualMachineSnapshot), err
}

// Delete takes name of the virtualMachineSnapshot and deletes it. Returns an error if one occurs.
func (c *FakeVirtualMachineSnapshots) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteActionWithOptions(virtualmachinesnapshotsResource, c.ns, name, opts), &v1alpha2.VirtualMachineSnapshot{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeVirtualMachineSnapshots) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(virtualmachinesnapshotsResource, c.ns, listOpts)

	_, err := c.Fake.Invokes(action, &v1alpha2.VirtualMachineSnapshotList{})
	return err
}

// Patch applies the patch and returns the patched virtualMachineSnapshot.
func (c *FakeVirtualMachineSnapshots) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha2.VirtualMachineSnapshot, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(virtualmachinesnapshotsResource, c.ns, name, pt, data, subresources...), &v1alpha2.VirtualMachineSnapshot{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha2.VirtualMachineSnapshot), err
}
