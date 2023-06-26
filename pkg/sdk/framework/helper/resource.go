package helper

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Object[T, ST any] interface {
	comparable
	client.Object
	DeepCopy() T
	GetObjectMeta() metav1.ObjectMeta
	GetStatus() ST
}

type Resource[T Object[T, ST], ST any] struct {
	name       types.NamespacedName
	ref        T
	mutatedRef T

	log    logr.Logger
	client client.Client
}

func NewResource[T Object[T, ST], ST any](name types.NamespacedName, log logr.Logger, client client.Client) *Resource[T, ST] {
	return &Resource[T, ST]{
		name:   name,
		log:    log,
		client: client,
	}
}

func (r *Resource[T, ST]) Name() types.NamespacedName {
	return r.name
}

func (r *Resource[T, ST]) Fetch(ctx context.Context, inObj T) error {
	obj, err := FetchObject(ctx, r.name, r.client, inObj)
	if err != nil {
		return err
	}

	r.ref = obj
	r.mutatedRef = obj.DeepCopy()
	return nil
}

func (r *Resource[T, ST]) IsFound() bool {
	var empty T
	return r.ref != empty
}

func (r *Resource[T, ST]) IsStatusChanged() bool {
	return !reflect.DeepEqual(r.ref.GetStatus(), r.mutatedRef.GetStatus())
}

func (r *Resource[T, ST]) Read() T {
	return r.ref
}

func (r *Resource[T, ST]) Write() T {
	return r.mutatedRef
}

func (r *Resource[T, ST]) UpdateMeta(ctx context.Context) error {
	if !r.IsFound() {
		return nil
	}
	if !reflect.DeepEqual(r.ref.GetObjectMeta(), r.mutatedRef.GetObjectMeta()) {
		if !reflect.DeepEqual(r.ref.GetStatus(), r.mutatedRef.GetStatus()) {
			return fmt.Errorf("status update is not allowed in the meta updater")
		}
		return r.client.Update(ctx, r.mutatedRef)
	}
	return nil
}

func (r *Resource[T, ST]) UpdateStatus(ctx context.Context) error {
	if !r.IsFound() {
		return nil
	}
	if !reflect.DeepEqual(r.ref.GetObjectMeta(), r.mutatedRef.GetObjectMeta()) {
		return fmt.Errorf("meta update is not allowed in the status updater")
	}
	if !reflect.DeepEqual(r.ref.GetStatus(), r.mutatedRef.GetStatus()) {
		return r.client.Status().Update(ctx, r.mutatedRef)
	}
	return nil
}
