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

package v1alpha2

import (
	"context"
	"time"

	scheme "github.com/deckhouse/virtualization/api/client/generated/clientset/versioned/scheme"
	v1alpha2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	rest "k8s.io/client-go/rest"
)

// VirtualMachineClassesGetter has a method to return a VirtualMachineClassInterface.
// A group's client should implement this interface.
type VirtualMachineClassesGetter interface {
	VirtualMachineClasses() VirtualMachineClassInterface
}

// VirtualMachineClassInterface has methods to work with VirtualMachineClass resources.
type VirtualMachineClassInterface interface {
	Create(ctx context.Context, virtualMachineClass *v1alpha2.VirtualMachineClass, opts v1.CreateOptions) (*v1alpha2.VirtualMachineClass, error)
	Update(ctx context.Context, virtualMachineClass *v1alpha2.VirtualMachineClass, opts v1.UpdateOptions) (*v1alpha2.VirtualMachineClass, error)
	UpdateStatus(ctx context.Context, virtualMachineClass *v1alpha2.VirtualMachineClass, opts v1.UpdateOptions) (*v1alpha2.VirtualMachineClass, error)
	Delete(ctx context.Context, name string, opts v1.DeleteOptions) error
	DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error
	Get(ctx context.Context, name string, opts v1.GetOptions) (*v1alpha2.VirtualMachineClass, error)
	List(ctx context.Context, opts v1.ListOptions) (*v1alpha2.VirtualMachineClassList, error)
	Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error)
	Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha2.VirtualMachineClass, err error)
	VirtualMachineClassExpansion
}

// virtualMachineClasses implements VirtualMachineClassInterface
type virtualMachineClasses struct {
	client rest.Interface
}

// newVirtualMachineClasses returns a VirtualMachineClasses
func newVirtualMachineClasses(c *VirtualizationV1alpha2Client) *virtualMachineClasses {
	return &virtualMachineClasses{
		client: c.RESTClient(),
	}
}

// Get takes name of the virtualMachineClass, and returns the corresponding virtualMachineClass object, and an error if there is any.
func (c *virtualMachineClasses) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1alpha2.VirtualMachineClass, err error) {
	result = &v1alpha2.VirtualMachineClass{}
	err = c.client.Get().
		Resource("virtualmachineclasses").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do(ctx).
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of VirtualMachineClasses that match those selectors.
func (c *virtualMachineClasses) List(ctx context.Context, opts v1.ListOptions) (result *v1alpha2.VirtualMachineClassList, err error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	result = &v1alpha2.VirtualMachineClassList{}
	err = c.client.Get().
		Resource("virtualmachineclasses").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Do(ctx).
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested virtualMachineClasses.
func (c *virtualMachineClasses) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	opts.Watch = true
	return c.client.Get().
		Resource("virtualmachineclasses").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Watch(ctx)
}

// Create takes the representation of a virtualMachineClass and creates it.  Returns the server's representation of the virtualMachineClass, and an error, if there is any.
func (c *virtualMachineClasses) Create(ctx context.Context, virtualMachineClass *v1alpha2.VirtualMachineClass, opts v1.CreateOptions) (result *v1alpha2.VirtualMachineClass, err error) {
	result = &v1alpha2.VirtualMachineClass{}
	err = c.client.Post().
		Resource("virtualmachineclasses").
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(virtualMachineClass).
		Do(ctx).
		Into(result)
	return
}

// Update takes the representation of a virtualMachineClass and updates it. Returns the server's representation of the virtualMachineClass, and an error, if there is any.
func (c *virtualMachineClasses) Update(ctx context.Context, virtualMachineClass *v1alpha2.VirtualMachineClass, opts v1.UpdateOptions) (result *v1alpha2.VirtualMachineClass, err error) {
	result = &v1alpha2.VirtualMachineClass{}
	err = c.client.Put().
		Resource("virtualmachineclasses").
		Name(virtualMachineClass.Name).
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(virtualMachineClass).
		Do(ctx).
		Into(result)
	return
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *virtualMachineClasses) UpdateStatus(ctx context.Context, virtualMachineClass *v1alpha2.VirtualMachineClass, opts v1.UpdateOptions) (result *v1alpha2.VirtualMachineClass, err error) {
	result = &v1alpha2.VirtualMachineClass{}
	err = c.client.Put().
		Resource("virtualmachineclasses").
		Name(virtualMachineClass.Name).
		SubResource("status").
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(virtualMachineClass).
		Do(ctx).
		Into(result)
	return
}

// Delete takes name of the virtualMachineClass and deletes it. Returns an error if one occurs.
func (c *virtualMachineClasses) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	return c.client.Delete().
		Resource("virtualmachineclasses").
		Name(name).
		Body(&opts).
		Do(ctx).
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *virtualMachineClasses) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	var timeout time.Duration
	if listOpts.TimeoutSeconds != nil {
		timeout = time.Duration(*listOpts.TimeoutSeconds) * time.Second
	}
	return c.client.Delete().
		Resource("virtualmachineclasses").
		VersionedParams(&listOpts, scheme.ParameterCodec).
		Timeout(timeout).
		Body(&opts).
		Do(ctx).
		Error()
}

// Patch applies the patch and returns the patched virtualMachineClass.
func (c *virtualMachineClasses) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha2.VirtualMachineClass, err error) {
	result = &v1alpha2.VirtualMachineClass{}
	err = c.client.Patch(pt).
		Resource("virtualmachineclasses").
		Name(name).
		SubResource(subresources...).
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(data).
		Do(ctx).
		Into(result)
	return
}
