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

package validator

import (
	"context"

	"k8s.io/apimachinery/pkg/api/equality"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	commonvm "github.com/deckhouse/virtualization-controller/pkg/common/vm"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// PeerMigrationLockValidator rejects a size change of a disk whose owner VM moves to or from a peer
// cluster: the peer already has the spec, so growing the disk here desynchronises the copy on the
// receiver (or the source, for a machine being received).
type PeerMigrationLockValidator struct {
	client client.Client
}

func NewPeerMigrationLockValidator(c client.Client) *PeerMigrationLockValidator {
	return &PeerMigrationLockValidator{client: c}
}

func (v *PeerMigrationLockValidator) ValidateCreate(_ context.Context, _ *v1alpha2.VirtualDisk) (admission.Warnings, error) {
	return nil, nil
}

func (v *PeerMigrationLockValidator) ValidateUpdate(ctx context.Context, oldVD, newVD *v1alpha2.VirtualDisk) (admission.Warnings, error) {
	if equality.Semantic.DeepEqual(oldVD.Spec.PersistentVolumeClaim.Size, newVD.Spec.PersistentVolumeClaim.Size) {
		return nil, nil
	}

	if err := commonvm.PeerMigrationLockErrorForDisk(ctx, v.client, newVD.Name, newVD.Namespace); err != nil {
		return nil, err
	}

	return nil, nil
}
