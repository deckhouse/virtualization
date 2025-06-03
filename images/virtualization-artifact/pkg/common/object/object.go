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

package object

import (
	"context"
	"fmt"
	"reflect"
	"time"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/patch"
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

func PurgeObject(ctx context.Context, client client.Client, obj client.Object) (bool, error) {
	key := types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}

	if len(obj.GetFinalizers()) > 0 {
		obj.SetFinalizers([]string{})

		err := client.Update(ctx, obj)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				return false, nil
			}
			return false, fmt.Errorf("remove finalizers for %s during cleanup: %w", key, err)
		}
	}

	err := client.Delete(ctx, obj)
	switch {
	case err == nil:
		return true, nil
	case k8serrors.IsNotFound(err):
		return false, nil
	default:
		return false, fmt.Errorf("delete object %s during cleanup: %w", key, err)
	}
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

// ShouldCleanupSubResources returns whether sub resources should be deleted:
// - CVMI, VMI has no annotation to retain pod after import
// - CVMI, VMI is deleted
func ShouldCleanupSubResources(obj metav1.Object) bool {
	return obj.GetAnnotations()[annotations.AnnPodRetainAfterCompletion] != "true" || obj.GetDeletionTimestamp() != nil
}

func IsTerminating(obj client.Object) bool {
	return !reflect.ValueOf(obj).IsNil() && obj.GetDeletionTimestamp() != nil
}

func AnyTerminating(objs ...client.Object) bool {
	for _, obj := range objs {
		if IsTerminating(obj) {
			return true
		}
	}

	return false
}

func NamespacedName(obj client.Object) types.NamespacedName {
	return types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}
}

func EnsureAnnotation(ctx context.Context, cl client.Client, obj client.Object, annoKey, annoValue string) error {
	op := patch.PatchAddOp
	if value, exists := obj.GetAnnotations()[annoKey]; exists {
		if value == annoValue {
			return nil
		}
		op = patch.PatchReplaceOp
	}
	jsonOp := patch.NewJSONPatchOperation(op, fmt.Sprintf("/metadata/annotations/%s", patch.EscapeJSONPointer(annoKey)), annoValue)
	bytes, err := patch.NewJSONPatch(jsonOp).Bytes()
	if err != nil {
		return err
	}

	return cl.Patch(ctx, obj, client.RawPatch(types.JSONPatchType, bytes))
}

func RemoveAnnotation(ctx context.Context, cl client.Client, obj client.Object, annoKey string) error {
	if _, exist := obj.GetAnnotations()[annoKey]; !exist {
		return nil
	}
	jsonOp := patch.WithRemove(fmt.Sprintf("/metadata/annotations/%s", patch.EscapeJSONPointer(annoKey)))
	bytes, err := patch.NewJSONPatch(jsonOp).Bytes()
	if err != nil {
		return err
	}
	return cl.Patch(ctx, obj, client.RawPatch(types.JSONPatchType, bytes))
}
