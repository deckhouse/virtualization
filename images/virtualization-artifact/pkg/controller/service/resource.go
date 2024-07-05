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

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/strings/slices"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
)

type ResourceObject[T, ST any] interface {
	comparable
	client.Object
	DeepCopy() T
	GetObjectMeta() metav1.Object
}

type ObjectStatusGetter[T, ST any] func(obj T) ST

type ObjectFactory[T any] func() T

type Resource[T ResourceObject[T, ST], ST any] struct {
	name       types.NamespacedName
	currentObj T
	changedObj T
	emptyObj   T

	objFactory      ObjectFactory[T]
	objStatusGetter ObjectStatusGetter[T, ST]
	client          client.Client
}

func NewResource[T ResourceObject[T, ST], ST any](name types.NamespacedName, client client.Client, objFactory ObjectFactory[T], objStatusGetter ObjectStatusGetter[T, ST]) *Resource[T, ST] {
	return &Resource[T, ST]{
		name:            name,
		client:          client,
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
	currentObj, err := helper.FetchObject(ctx, r.name, r.client, r.objFactory())
	if err != nil {
		return err
	}

	r.currentObj = currentObj
	if r.IsEmpty() {
		r.changedObj = r.emptyObj
		return nil
	}

	r.changedObj = currentObj.DeepCopy()
	return nil
}

func (r *Resource[T, ST]) IsEmpty() bool {
	return r.currentObj == r.emptyObj
}

func (r *Resource[T, ST]) Current() T {
	return r.currentObj
}

func (r *Resource[T, ST]) Changed() T {
	return r.changedObj
}

func (r *Resource[T, ST]) Update(ctx context.Context) error {
	if r.IsEmpty() {
		return nil
	}

	finalizers := r.changedObj.GetFinalizers()

	if !reflect.DeepEqual(r.getObjStatus(r.currentObj), r.getObjStatus(r.changedObj)) {
		if err := r.client.Status().Update(ctx, r.changedObj); err != nil {
			return fmt.Errorf("error updating status subresource: %w", err)
		}
		r.changedObj.SetFinalizers(finalizers)
	}

	if !slices.Equal(r.currentObj.GetFinalizers(), r.changedObj.GetFinalizers()) {
		patch, err := GetPatchFinalizers(r.changedObj.GetFinalizers())
		if err != nil {
			return err
		}

		if err = r.client.Patch(ctx, r.changedObj, patch); err != nil {
			return fmt.Errorf("error updating: %w", err)
		}
	}

	return nil
}

func GetPatchOwnerReferences(ownerReferences []metav1.OwnerReference) (client.Patch, error) {
	data, err := json.Marshal(map[string]interface{}{
		"metadata": map[string]interface{}{
			"ownerReferences": ownerReferences,
		},
	})
	if err != nil {
		return nil, err
	}

	return client.RawPatch(types.MergePatchType, data), nil
}

func GetPatchFinalizers(finalizers []string) (client.Patch, error) {
	data, err := json.Marshal(map[string]interface{}{
		"metadata": map[string]interface{}{
			"finalizers": finalizers,
		},
	})
	if err != nil {
		return nil, err
	}

	return client.RawPatch(types.MergePatchType, data), nil
}

func MergeResults(results ...reconcile.Result) reconcile.Result {
	var result reconcile.Result
	for _, r := range results {
		if r.IsZero() {
			continue
		}
		if r.Requeue && r.RequeueAfter == 0 {
			return r
		}
		if result.IsZero() && r.RequeueAfter > 0 {
			result = r
			continue
		}
		if r.RequeueAfter > 0 && r.RequeueAfter < result.RequeueAfter {
			result.RequeueAfter = r.RequeueAfter
		}
	}
	return result
}
