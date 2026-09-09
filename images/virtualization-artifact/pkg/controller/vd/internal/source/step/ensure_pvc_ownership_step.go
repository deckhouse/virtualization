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

package step

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// EnsurePVCOwnershipStep adopts a target PVC that was created without an ownerReference back to this
// VirtualDisk, and puts the deletion-protection finalizer on it.
//
// This is needed for the unified-snapshotter restore path: the PVC materialized for a
// VolumeRestoreRequest is created out of band by storage-foundation, never by us — see
// PersistentVolumeClaimService.EnsureVolumeRestoreRequest. The VRR itself is a one-shot resource
// storage-foundation's own GC eventually reaps; nothing downstream should have to know it ever existed.
// But other code (our own PersistentVolumeClaimWatcher, and any other controller that walks
// PVC -> VirtualDisk) expects every VirtualDisk's target PVC to carry a direct ownerReference to it.
//
// A no-op whenever the PVC doesn't exist yet, is terminating, or already carries both — in particular for
// every OTHER data source, whose PVC-creation code sets ownerReference and finalizer at creation time.
type EnsurePVCOwnershipStep struct {
	pvc        *corev1.PersistentVolumeClaim
	finalizers []string
	client     client.Client
}

func NewEnsurePVCOwnershipStep(pvc *corev1.PersistentVolumeClaim, finalizers []string, client client.Client) *EnsurePVCOwnershipStep {
	return &EnsurePVCOwnershipStep{
		pvc:        pvc,
		finalizers: finalizers,
		client:     client,
	}
}

func (s EnsurePVCOwnershipStep) Take(ctx context.Context, vd *v1alpha2.VirtualDisk) (*reconcile.Result, error) {
	if s.pvc == nil || object.IsTerminating(s.pvc) {
		return nil, nil
	}

	adopted, foreignController := false, false
	for _, ref := range s.pvc.OwnerReferences {
		if ref.Kind == v1alpha2.VirtualDiskKind && ref.UID == vd.UID {
			adopted = true
			continue
		}
		if ref.Controller != nil && *ref.Controller {
			foreignController = true
		}
	}

	missingFinalizers := s.missingFinalizers()
	if adopted && len(missingFinalizers) == 0 {
		return nil, nil
	}

	patched := s.pvc.DeepCopy()
	if !adopted {
		ownerRef := service.MakeControllerOwnerReference(vd)
		if foreignController {
			ownerRef.Controller = nil
			ownerRef.BlockOwnerDeletion = nil
		}
		patched.OwnerReferences = append(patched.OwnerReferences, ownerRef)
	}
	patched.Finalizers = append(patched.Finalizers, missingFinalizers...)

	if err := s.client.Patch(ctx, patched, client.StrategicMergeFrom(s.pvc)); err != nil {
		return nil, fmt.Errorf("adopt restored pvc %q: %w", s.pvc.Name, err)
	}
	return nil, nil
}

func (s EnsurePVCOwnershipStep) missingFinalizers() []string {
	present := make(map[string]struct{}, len(s.pvc.Finalizers))
	for _, f := range s.pvc.Finalizers {
		present[f] = struct{}{}
	}

	var missing []string
	for _, f := range s.finalizers {
		if _, ok := present[f]; !ok {
			missing = append(missing, f)
		}
	}
	return missing
}
