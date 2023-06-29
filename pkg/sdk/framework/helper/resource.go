package helper

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/cache"
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
	currentObj T
	changedObj T

	allocatedObj T
	emptyObj     T

	log    logr.Logger
	client client.Client
	cache  cache.Cache
}

func NewResource[T Object[T, ST], ST any](name types.NamespacedName, log logr.Logger, client client.Client, cache cache.Cache, allocatedObj T) *Resource[T, ST] {
	return &Resource[T, ST]{
		name:         name,
		log:          log,
		client:       client,
		cache:        cache,
		allocatedObj: allocatedObj,
	}
}

func (r *Resource[T, ST]) Name() types.NamespacedName {
	return r.name
}

func (r *Resource[T, ST]) Fetch(ctx context.Context) error {
	//if !r.cache.WaitForCacheSync(ctx) {
	//	return fmt.Errorf("unable to wait for cache sync")
	//}

	currentObj, err := FetchObject(ctx, r.name, r.client, r.allocatedObj.DeepCopy())
	if err != nil {
		return err
	}
	r.log.Info("Resource.Fetch", "name", r.name, "obj", currentObj, "status", currentObj.GetStatus())

	r.currentObj = currentObj
	if r.IsEmpty() {
		r.changedObj = r.emptyObj
	} else {
		r.changedObj = currentObj.DeepCopy()
	}
	return nil
}

func (r *Resource[T, ST]) IsEmpty() bool {
	return r.currentObj == r.emptyObj
}

func (r *Resource[T, ST]) IsStatusChanged() bool {
	return !reflect.DeepEqual(r.currentObj.GetStatus(), r.changedObj.GetStatus())
}

func (r *Resource[T, ST]) Current() T {
	return r.currentObj
}

func (r *Resource[T, ST]) Changed() T {
	return r.changedObj
}

func (r *Resource[T, ST]) UpdateMeta(ctx context.Context) error {
	if r.IsEmpty() {
		return nil
	}
	if !reflect.DeepEqual(r.currentObj.GetStatus(), r.changedObj.GetStatus()) {
		return fmt.Errorf("status update is not allowed in the meta updater: %#v changed to %#v", r.currentObj.GetStatus(), r.changedObj.GetStatus())
	}
	if !reflect.DeepEqual(r.currentObj.GetObjectMeta(), r.changedObj.GetObjectMeta()) {
		if err := r.client.Update(ctx, r.changedObj); err != nil {
			return fmt.Errorf("error updating: %w", err)
		}
		if err := r.Fetch(ctx); err != nil {
			return fmt.Errorf("error fetching object after update: %w", err)
		}
	}
	return nil
}

func (r *Resource[T, ST]) UpdateStatus(ctx context.Context) error {
	if r.IsEmpty() {
		return nil
	}

	r.log.Info("UpdateStatus obj before status update", "currentObj.Status", r.currentObj.GetStatus(), "changedObj.Status", r.changedObj.GetStatus())
	if !reflect.DeepEqual(r.currentObj.GetObjectMeta(), r.changedObj.GetObjectMeta()) {
		return fmt.Errorf("meta update is not allowed in the status updater: %#v changed to %#v", r.currentObj.GetObjectMeta(), r.changedObj.GetObjectMeta())
	}
	if !reflect.DeepEqual(r.currentObj.GetStatus(), r.changedObj.GetStatus()) {
		if err := r.client.Status().Update(ctx, r.changedObj); err != nil {
			return fmt.Errorf("error updating: %w", err)
		}
		if err := r.client.Update(ctx, r.changedObj); err != nil {
			return fmt.Errorf("error updating: %w", err)
		}
		if err := r.Fetch(ctx); err != nil {
			return fmt.Errorf("error refreshing object after update: %w", err)
		}
		r.log.Info("UpdateStatus obj after status update", "currentObj.Status", r.currentObj.GetStatus(), "changedObj.Status", r.changedObj.GetStatus())
	}
	return nil
}
