package helper

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Object[T, ST any] interface {
	comparable
	client.Object
	DeepCopy() T
	GetObjectMeta() metav1.Object
}

type ObjectStatusGetter[T, ST any] func(obj T) ST

type ObjectFactory[T any] func() T

type Resource[T Object[T, ST], ST any] struct {
	name       types.NamespacedName
	currentObj T
	changedObj T
	emptyObj   T

	objFactory      ObjectFactory[T]
	objStatusGetter ObjectStatusGetter[T, ST]
	log             logr.Logger
	client          client.Client
	cache           cache.Cache
}

func NewResource[T Object[T, ST], ST any](name types.NamespacedName, log logr.Logger, client client.Client, cache cache.Cache, objFactory ObjectFactory[T], objStatusGetter ObjectStatusGetter[T, ST]) *Resource[T, ST] {
	return &Resource[T, ST]{
		name:            name,
		log:             log,
		client:          client,
		cache:           cache,
		objFactory:      objFactory,
		objStatusGetter: objStatusGetter,
	}
}

func (r *Resource[T, ST]) getObjStatus(obj T) (ret ST) {
	if obj != r.emptyObj {
		ret = r.objStatusGetter(obj)
	}
	return
}

func (r *Resource[T, ST]) Name() types.NamespacedName {
	return r.name
}

func (r *Resource[T, ST]) Fetch(ctx context.Context) error {
	currentObj, err := FetchObject(ctx, r.name, r.client, r.objFactory())
	if err != nil {
		return err
	}
	r.log.V(2).Info("Resource.Fetch", "name", r.name, "obj", currentObj, "status", r.getObjStatus(currentObj))

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
	return !reflect.DeepEqual(r.getObjStatus(r.currentObj), r.getObjStatus(r.changedObj))
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
	if !reflect.DeepEqual(r.getObjStatus(r.currentObj), r.getObjStatus(r.changedObj)) {
		return fmt.Errorf("status update is not allowed in the meta updater: %#v changed to %#v", r.getObjStatus(r.currentObj), r.getObjStatus(r.changedObj))
	}
	if !reflect.DeepEqual(r.currentObj.GetObjectMeta(), r.changedObj.GetObjectMeta()) {
		if err := r.client.Update(ctx, r.changedObj); err != nil {
			return fmt.Errorf("error updating: %w", err)
		}
		r.log.V(2).Info("UpdateMeta object updated", "currentObj.ObjectMeta", r.currentObj.GetObjectMeta(), "changedObj.ObjectMeta", r.changedObj.GetObjectMeta())
		r.currentObj = r.changedObj
	}
	return nil
}

func (r *Resource[T, ST]) UpdateStatus(ctx context.Context) error {
	if r.IsEmpty() {
		return nil
	}

	r.log.Info("UpdateStatus obj before status update", "currentObj.Status", r.getObjStatus(r.currentObj), "changedObj.Status", r.getObjStatus(r.changedObj))
	if !reflect.DeepEqual(r.currentObj.GetObjectMeta(), r.changedObj.GetObjectMeta()) {
		return fmt.Errorf("meta update is not allowed in the status updater: %#v changed to %#v", r.currentObj.GetObjectMeta(), r.changedObj.GetObjectMeta())
	}
	if !reflect.DeepEqual(r.getObjStatus(r.currentObj), r.getObjStatus(r.changedObj)) {
		if err := r.client.Status().Update(ctx, r.changedObj); err != nil {
			return fmt.Errorf("error updating: %w", err)
		}
		if err := r.client.Update(ctx, r.changedObj); err != nil {
			return fmt.Errorf("error updating: %w", err)
		}
		r.currentObj = r.changedObj

		r.log.V(2).Info("UpdateStatus obj after status update", "currentObj.Status", r.getObjStatus(r.currentObj), "changedObj.Status", r.getObjStatus(r.changedObj))
	}
	return nil
}
