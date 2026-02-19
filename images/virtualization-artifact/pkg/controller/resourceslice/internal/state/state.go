/*
Copyright 2026 Flant JSC

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

package state

import (
	"context"
	"fmt"

	resourcev1 "k8s.io/api/resource/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
)

const (
	draDriverName = "virtualization-usb"
)

type ResourceSliceState interface {
	ResourceSlice() *resourcev1.ResourceSlice
	ResourceSlices(ctx context.Context) ([]resourcev1.ResourceSlice, error)
}

func New(client client.Client, resourceSlice *resourcev1.ResourceSlice) ResourceSliceState {
	return &resourceSliceState{
		client:        client,
		resourceSlice: resourceSlice,
	}
}

type resourceSliceState struct {
	client        client.Client
	resourceSlice *resourcev1.ResourceSlice
}

func (s *resourceSliceState) ResourceSlice() *resourcev1.ResourceSlice {
	return s.resourceSlice
}

func (s *resourceSliceState) ResourceSlices(ctx context.Context) ([]resourcev1.ResourceSlice, error) {
	var slices resourcev1.ResourceSliceList
	if err := s.client.List(ctx, &slices, client.MatchingFields{indexer.IndexFieldResourceSliceByDriver: draDriverName}); err != nil {
		return nil, fmt.Errorf("failed to list ResourceSlices: %w", err)
	}

	return slices.Items, nil
}
