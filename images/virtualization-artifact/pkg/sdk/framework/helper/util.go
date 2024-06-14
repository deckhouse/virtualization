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

package helper

import (
	"context"
	"fmt"
	"reflect"
	"time"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func FetchObject[T client.Object](ctx context.Context, key types.NamespacedName, client client.Client, obj T, opts ...client.GetOption) (T, error) {
	if err := client.Get(ctx, key, obj, opts...); err != nil {
		var empty T
		if k8serrors.IsNotFound(err) {
			return empty, nil
		}
		return empty, err
	}
	return obj, nil
}

func DeleteObject[T client.Object](ctx context.Context, client client.Client, obj T, opts ...client.DeleteOption) error {
	var empty T
	if reflect.DeepEqual(empty, obj) || obj.GetDeletionTimestamp() != nil {
		return nil
	}
	err := client.Delete(ctx, obj, opts...)
	if err != nil && k8serrors.IsNotFound(err) {
		return nil
	}
	return err
}

// CleanupObject removes finalizers on object (if any) and then delete object.
// obj must be a struct pointer so that obj can be updated with the response returned by the Server.
func CleanupObject(ctx context.Context, client client.Client, obj client.Object) error {
	key := types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}
	if len(obj.GetFinalizers()) > 0 {
		obj.SetFinalizers([]string{})

		err := client.Update(ctx, obj)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				return nil
			}
			return fmt.Errorf("remove finalizers for %s during cleanup: %w", key, err)
		}
	}

	err := client.Delete(ctx, obj)
	if err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("delete object %s during cleanup: %w", key, err)
	}

	return nil
}

// CleanupByName searches object by its name, removes finalizers on object (if any) and then delete object.
// obj must be a struct pointer so that obj can be updated with the response returned by the Server.
func CleanupByName(ctx context.Context, client client.Client, key client.ObjectKey, obj client.Object) error {
	err := client.Get(ctx, key, obj)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("get object %s during cleanup: %w", key, err)
	}

	return CleanupObject(ctx, client, obj)
}

// GetAge returns the age of an object.
func GetAge(obj client.Object) time.Duration {
	return time.Since(obj.GetCreationTimestamp().Time).Truncate(time.Second)
}
