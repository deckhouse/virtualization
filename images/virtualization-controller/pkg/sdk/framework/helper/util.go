package helper

import (
	"context"

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
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return err
}
