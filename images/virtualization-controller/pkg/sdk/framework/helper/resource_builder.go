package helper

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/util"
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
	newOwnerRefs := util.SetArrayElem(
		b.Resource.GetOwnerReferences(),
		*metav1.NewControllerRef(obj, gvk),
		func(v1, v2 metav1.OwnerReference) bool {
			return v1.Name == v2.Name
		}, false,
	)
	b.Resource.SetOwnerReferences(newOwnerRefs)
}

func (b *ResourceBuilder[T]) AddAnnotation(annotation, value string) {
	common.AddAnnotation(b.Resource, annotation, value)
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
