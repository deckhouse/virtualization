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

package step

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type SourceUIDGetter func(ctx context.Context, vi *virtv2.VirtualImage) (*types.UID, error)

type SetSourceUIDStep struct {
	getSourceUID SourceUIDGetter
}

func NewSetSourceUIDStep(getSourceUID SourceUIDGetter) *SetSourceUIDStep {
	return &SetSourceUIDStep{
		getSourceUID: getSourceUID,
	}
}

func (s SetSourceUIDStep) Take(ctx context.Context, vi *virtv2.VirtualImage) (*reconcile.Result, error) {
	var err error
	vi.Status.SourceUID, err = s.getSourceUID(ctx, vi)
	if err != nil {
		return nil, err
	}

	return nil, nil
}
