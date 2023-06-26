package helper

import (
	"context"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func FetchObject[T client.Object](ctx context.Context, key types.NamespacedName, client client.Client, obj T) (T, error) {
	if err := client.Get(ctx, key, obj); err != nil {
		var empty T
		if k8serrors.IsNotFound(err) {
			return empty, nil
		}
		return empty, err
	}
	return obj, nil
}
