package service

import (
	"context"

	storev1 "k8s.io/api/storage/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
)

func GetDefaultStorageClass(ctx context.Context, cl client.Client) (*storev1.StorageClass, error) {
	var scs storev1.StorageClassList
	err := cl.List(ctx, &scs, &client.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, sc := range scs.Items {
		if sc.Annotations[common.AnnDefaultStorageClass] == "true" {
			return &sc, nil
		}
	}

	return nil, ErrDefaultStorageClassNotFound
}
