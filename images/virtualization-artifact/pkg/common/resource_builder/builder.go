/*
Copyright 2024 Flant JSC

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

package resource_builder //nolint:stylecheck // we don't care

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/array"
)

type ResourceBuilderOptions struct {
	ResourceExists bool
}

type ResourceBuilder[T client.Object] struct {
	ResourceBuilderOptions
	Resource T
}

func NewResourceBuilder[T client.Object](resource T, opts ResourceBuilderOptions) ResourceBuilder[T] {
	return ResourceBuilder[T]{
		ResourceBuilderOptions: opts,
		Resource:               resource,
	}
}

func (b *ResourceBuilder[T]) SetOwnerRef(obj metav1.Object, gvk schema.GroupVersionKind) {
	SetOwnerRef(b.Resource, *metav1.NewControllerRef(obj, gvk))
}

func (b *ResourceBuilder[T]) AddAnnotation(annotation, value string) {
	annotations.AddAnnotation(b.Resource, annotation, value)
}

func (b *ResourceBuilder[T]) AddFinalizer(finalizer string) {
	controllerutil.AddFinalizer(b.Resource, finalizer)
}

func (b *ResourceBuilder[T]) GetResource() T {
	return b.Resource
}

func (b *ResourceBuilder[T]) IsResourceExists() bool {
	return b.ResourceExists
}

func SetOwnerRef(obj metav1.Object, ref metav1.OwnerReference) bool {
	newOwnerRefs := array.SetArrayElem(
		obj.GetOwnerReferences(),
		ref,
		func(v1, v2 metav1.OwnerReference) bool {
			return v1.Name == v2.Name
		}, false,
	)

	if len(newOwnerRefs) == len(obj.GetOwnerReferences()) {
		return false
	}

	obj.SetOwnerReferences(newOwnerRefs)
	return true
}
