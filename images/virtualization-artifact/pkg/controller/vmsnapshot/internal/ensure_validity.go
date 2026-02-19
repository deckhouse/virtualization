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

package internal

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// EnsureValidityHandler ensure vmsnapshot validity before update is called:
// - Ensure phase is not empty. Empty phase prevents object update.
// - More validations TBD ...
type EnsureValidityHandler struct{}

func NewEnsureValidityHandler() *EnsureValidityHandler {
	return &EnsureValidityHandler{}
}

func (h EnsureValidityHandler) Handle(_ context.Context, vmSnapshot *v1alpha2.VirtualMachineSnapshot) (reconcile.Result, error) {
	if vmSnapshot.Status.Phase == "" {
		vmSnapshot.Status.Phase = v1alpha2.VirtualMachineSnapshotPhasePending
	}
	return reconcile.Result{}, nil
}
