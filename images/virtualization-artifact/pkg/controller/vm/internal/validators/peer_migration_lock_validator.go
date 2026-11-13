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

package validators

import (
	"context"

	"k8s.io/apimachinery/pkg/api/equality"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	commonvm "github.com/deckhouse/virtualization-controller/pkg/common/vm"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// PeerMigrationLockValidator keeps the spec of a VM that moves to a peer cluster, or arrives from one,
// as it is until the move ends: the peer received that spec. Only the controller of this module changes it
// then: that is how the move hands the VM back with the Manual run policy.
type PeerMigrationLockValidator struct {
	client client.Client
}

func NewPeerMigrationLockValidator(c client.Client) *PeerMigrationLockValidator {
	return &PeerMigrationLockValidator{client: c}
}

func (v *PeerMigrationLockValidator) ValidateCreate(_ context.Context, _ *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	return nil, nil
}

func (v *PeerMigrationLockValidator) ValidateUpdate(ctx context.Context, oldVM, newVM *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	if equality.Semantic.DeepEqual(oldVM.Spec, newVM.Spec) {
		return nil, nil
	}

	if req, err := admission.RequestFromContext(ctx); err == nil && req.UserInfo.Username == controllerUser {
		return nil, nil
	}

	return nil, commonvm.PeerMigrationLockError(ctx, v.client, newVM)
}

// controllerUser is the controller of this module: it hands a VM back while the move still locks it.
const controllerUser = "system:serviceaccount:d8-virtualization:virtualization-controller"
