/*
Copyright 2025 Flant JSC

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

package internal

import (
	"context"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type PeriodicEnqueuer struct {
	client client.Client
}

func NewPeriodicEnqueuer(client client.Client) *PeriodicEnqueuer {
	return &PeriodicEnqueuer{
		client: client,
	}
}

func (e *PeriodicEnqueuer) ListForDelete(ctx context.Context, now time.Time) ([]client.Object, error) {
	cviList := &v1alpha2.ClusterVirtualImageList{}
	if err := e.client.List(ctx, cviList); err != nil {
		return nil, err
	}

	objs := make([]client.Object, 0)

	for i := range cviList.Items {
		cvi := &cviList.Items[i]

		if cvi.Status.Phase != v1alpha2.ImageReady {
			continue
		}

		objs = append(objs, cvi)
	}

	return objs, nil
}
