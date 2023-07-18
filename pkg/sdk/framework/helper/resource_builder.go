package helper

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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

func (b *ResourceBuilder[T]) AddOwnerRef(obj metav1.Object, gvk schema.GroupVersionKind) {
	b.Resource.SetOwnerReferences(
		append(
			b.Resource.GetOwnerReferences(),
			*metav1.NewControllerRef(obj, gvk),
		),
	)
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
