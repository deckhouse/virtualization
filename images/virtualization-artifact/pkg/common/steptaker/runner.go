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

package steptaker

import (
	"context"
	"errors"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type Resource interface {
	*v1alpha2.VirtualDisk | *v1alpha2.VirtualImage | *v1alpha2.VirtualMachineIPAddress | *v1alpha2.VirtualMachineMACAddress | *v1alpha2.VirtualMachineOperation | *v1alpha2.VirtualMachineSnapshotOperation
}

type StepTaker[R Resource] interface {
	Take(ctx context.Context, obj R) (*reconcile.Result, error)
}

type StepTakers[R Resource] []StepTaker[R]

func NewStepTakers[R Resource](takers ...StepTaker[R]) StepTakers[R] {
	return takers
}

func (steps StepTakers[R]) Run(ctx context.Context, r R) (reconcile.Result, error) {
	for _, s := range steps {
		res, err := s.Take(ctx, r)
		if err != nil {
			return reconcile.Result{}, err
		}

		if res != nil {
			return *res, nil
		}
	}

	return reconcile.Result{}, errors.New("none of the steps returned a final result, please report a bug")
}
