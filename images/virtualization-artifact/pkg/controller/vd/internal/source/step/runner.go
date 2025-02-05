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
	"errors"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type Resource interface {
	*virtv2.VirtualDisk | *virtv2.VirtualImage
}

type Taker[R Resource] interface {
	Take(ctx context.Context, obj R) (*reconcile.Result, error)
}

type Takers[R Resource] []Taker[R]

func NewTakers[R Resource](takers ...Taker[R]) Takers[R] {
	return takers
}

func (steps Takers[R]) Run(ctx context.Context, r R) (reconcile.Result, error) {
	for _, s := range steps {
		res, err := s.Take(ctx, r)
		if err != nil {
			return reconcile.Result{}, err
		}

		if res != nil {
			return *res, nil
		}
	}

	return reconcile.Result{}, errors.New("todo unexpected")
}
