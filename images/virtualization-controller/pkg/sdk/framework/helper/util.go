package helper

import (
	"context"
	"fmt"

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

func DeleteObject(ctx context.Context, client client.Client, obj client.Object, opts ...client.DeleteOption) error {
	if obj == nil || obj.GetName() == "" {
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
