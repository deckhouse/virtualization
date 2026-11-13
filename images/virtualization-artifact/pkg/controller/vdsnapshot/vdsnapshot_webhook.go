/*
Copyright 2024 Flant JSC

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

package vdsnapshot

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/common/validate"
	commonvm "github.com/deckhouse/virtualization-controller/pkg/common/vm"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type Validator struct {
	logger                     *log.Logger
	client                     client.Client
	unifiedSnapshotterPresent  bool
	volumeSnapshotAPIAvailable bool
}

func NewValidator(logger *log.Logger, c client.Client, unifiedSnapshotterPresent, volumeSnapshotAPIAvailable bool) *Validator {
	return &Validator{
		logger:                     logger.With("webhook", "validator"),
		client:                     c,
		unifiedSnapshotterPresent:  unifiedSnapshotterPresent,
		volumeSnapshotAPIAvailable: volumeSnapshotAPIAvailable,
	}
}

func (v *Validator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	vds, ok := obj.(*v1alpha2.VirtualDiskSnapshot)
	if !ok {
		return nil, fmt.Errorf("expected a VirtualDiskSnapshot but got a %T", obj)
	}

	if err := validate.VolumeSnapshotAPIAvailable(vds, v.unifiedSnapshotterPresent, v.volumeSnapshotAPIAvailable); err != nil {
		return nil, err
	}

	if err := validate.UnifiedSnapshotterAnnotationAvailable(vds, v.unifiedSnapshotterPresent); err != nil {
		return nil, err
	}

	if err := validate.SnapshotterAnnotationsExclusive(vds); err != nil {
		return nil, err
	}

	if err := v.validatePeerMigrationLock(ctx, vds); err != nil {
		return nil, err
	}

	return nil, validate.ImportModeRequiresUnifiedSnapshotter(vds, vds.Spec.Mode, v.unifiedSnapshotterPresent)
}

// validatePeerMigrationLock rejects a snapshot of a virtual disk whose owning virtual machine is
// moving to or from a peer cluster: the snapshot would contain an incomplete copy of the disk.
func (v *Validator) validatePeerMigrationLock(ctx context.Context, vds *v1alpha2.VirtualDiskSnapshot) error {
	if vds.Spec.VirtualDiskName == "" {
		return nil
	}

	return commonvm.PeerMigrationLockErrorForDisk(ctx, v.client, vds.Spec.VirtualDiskName, vds.Namespace)
}

func (v *Validator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldVDS, ok := oldObj.(*v1alpha2.VirtualDiskSnapshot)
	if !ok {
		return nil, fmt.Errorf("expected an old VirtualDiskSnapshot but got a %T", newObj)
	}

	newVDS, ok := newObj.(*v1alpha2.VirtualDiskSnapshot)
	if !ok {
		return nil, fmt.Errorf("expected a new VirtualDiskSnapshot but got a %T", newObj)
	}

	v.logger.Info("Validating VirtualDiskSnapshot")

	if oldVDS.Generation != newVDS.Generation {
		return nil, fmt.Errorf("VirtualDiskSnapshot is an idempotent resource: specification changes are not available")
	}

	if err := validate.SnapshotterAnnotationsImmutable(oldVDS, newVDS); err != nil {
		return nil, err
	}

	return nil, nil
}

func (v *Validator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	err := fmt.Errorf("misconfigured webhook rules: delete operation not implemented")
	v.logger.Error("Ensure the correctness of ValidatingWebhookConfiguration", "err", err)

	return nil, nil
}
