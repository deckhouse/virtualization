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

// Package snapshotter decides which of the two snapshot mechanisms owns a given
// VirtualMachineSnapshot or VirtualDiskSnapshot.
//
// Every controller on the capture side routes through UseUnified, so the unified-snapshotter
// controllers and the built-in ones cannot disagree about who drives an object: their guards are the
// same expression, read in opposite directions.
//
// The restore side deliberately does not use this package. It reads what the object's status records
// about the capture that actually happened (restorer.IsUnifiedCapture and its VirtualDiskSnapshot
// counterpart), so snapshots taken before the cluster default moved stay restorable.
//
// NOTE: bool type is used intentionally as a temporary measure. We will remove all parts
// related to the built-in mechanism in the future.
package snapshotter

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// UseUnified reports whether the snapshot of obj must be taken by the unified-snapshotter SDK-based
// controllers (true) or by the built-in Secret-based ones (false).
//
// This is the metadata-only form, for callers that hold no more than object metadata — a
// controller-runtime predicate, say. Reconcilers hold the typed object and must use
// UseUnifiedForVirtualMachineSnapshot or UseUnifiedForVirtualDiskSnapshot instead: those also honour
// what the object's status records, which this form cannot see.
func UseUnified(obj metav1.Object, unifiedPresent bool) bool {
	if pinned, ok := pinnedMechanism(obj); ok {
		return pinned
	}

	return unifiedPresent
}

// UseUnifiedForVirtualMachineSnapshot routes a VirtualMachineSnapshot. It is the root of a snapshot
// tree, so there is no parent to inherit from.
//
// spec.mode: Import outranks everything below it, pins included. An import is materialized from an
// uploaded payload by the state-snapshotter core. No support from built-in mechanism for this mode.
// Pinning with "built-in" annotation is useless for mode: Import.
func UseUnifiedForVirtualMachineSnapshot(vms *v1alpha2.VirtualMachineSnapshot, unifiedPresent bool) bool {
	if vms == nil {
		return unifiedPresent
	}

	if vms.IsImport() {
		return true
	}
	if pinned, ok := pinnedMechanism(vms); ok {
		return pinned
	}
	builtIn := vms.Status.VirtualMachineSnapshotSecretName != "" ||
		phasePastPending(vms.Status.Phase, v1alpha2.VirtualMachineSnapshotPhasePending) ||
		pendingWithoutUnifiedFinalizer(vms)
	if captured, ok := capturedMechanism(vms.Status.CaptureState != nil, builtIn); ok {
		return captured
	}

	return unifiedPresent
}

// UseUnifiedForVirtualDiskSnapshot routes a VirtualDiskSnapshot. spec.mode: Import outranks everything
// else here too — see UseUnifiedForVirtualMachineSnapshot.
//
// A VirtualDiskSnapshot may be a child of a VirtualMachineSnapshot, and a child must never split from
// its parent: the two would take a disk twice, through both mechanisms at once. So before falling back
// to the cluster default, an uncaptured child adopts whatever its parent's status records.
//
// Deriving that from the parent's status rather than from an annotation the parent stamped is what makes
// a controller restart — or an upgrade that moves the default — survivable in the middle of a capture:
// the parent's status is the parent's own record, and it is already written by the time any child
// exists. The unified controller applies PhasePlanning, and so captureState, before EnsureChildren; the
// built-in one writes no captureState at all. For an object that has a parent, the presence of the
// parent's captureState therefore tells the two mechanisms apart exactly — and unlike the built-in
// parent's status.virtualMachineSnapshotSecretName, which only lands after the children are created, it
// is never late.
//
// reader should be uncached. A cached parent may not yet show the captureState its controller wrote
// moments before creating this child, and reading it stale would hand a unified child to the built-in
// controller, which would start taking a CSI VolumeSnapshot of the same disk.
func UseUnifiedForVirtualDiskSnapshot(ctx context.Context, reader client.Reader, vds *v1alpha2.VirtualDiskSnapshot, unifiedPresent bool) (bool, error) {
	if vds == nil {
		return unifiedPresent, nil
	}

	if vds.IsImport() {
		return true, nil
	}
	if pinned, ok := pinnedMechanism(vds); ok {
		return pinned, nil
	}
	builtIn := vds.Status.VolumeSnapshotName != "" ||
		controllerutil.ContainsFinalizer(vds, v1alpha2.FinalizerVDSnapshotCleanup) ||
		phasePastPending(vds.Status.Phase, v1alpha2.VirtualDiskSnapshotPhasePending)
	if captured, ok := capturedMechanism(vds.Status.CaptureState != nil, builtIn); ok {
		return captured, nil
	}

	parent, err := parentVirtualMachineSnapshot(ctx, reader, vds)
	if err != nil {
		return false, err
	}
	if parent != nil {
		return parent.Status.CaptureState != nil, nil
	}

	return unifiedPresent, nil
}

// parentVirtualMachineSnapshot returns the VirtualMachineSnapshot that owns vds, or nil when vds is
// standalone or its parent is already gone.
func parentVirtualMachineSnapshot(ctx context.Context, reader client.Reader, vds *v1alpha2.VirtualDiskSnapshot) (*v1alpha2.VirtualMachineSnapshot, error) {
	for _, ref := range vds.OwnerReferences {
		// Both mechanisms set this ownerReference, though only one of them marks it as the controller.
		if ref.Kind != v1alpha2.VirtualMachineSnapshotKind || ref.APIVersion != v1alpha2.SchemeGroupVersion.String() {
			continue
		}

		parent := &v1alpha2.VirtualMachineSnapshot{}
		key := types.NamespacedName{Namespace: vds.Namespace, Name: ref.Name}
		if err := reader.Get(ctx, key, parent); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, nil
			}
			return nil, fmt.Errorf("fetch the owning VirtualMachineSnapshot %q: %w", ref.Name, err)
		}

		return parent, nil
	}

	return nil, nil
}

// pinnedMechanism reports the mechanism an explicit annotation pins obj to, if any.
//
// A pin wins over everything else. Those annotations are immutable, so a pinned object was pinned
// before anything could have captured it and the two can never actually disagree.
// AnnUseBuiltInSnapshotter is read first, which settles an object carrying both — admission rejects
// that combination (validate.SnapshotterAnnotationsExclusive), so this only decides an already-invalid
// object, and pinning it to the mechanism that is always running beats pinning it to one that may not
// be.
//
// The annotations are transitional: they exist so an operator can override the cluster default while
// both mechanisms are still around, and they go away with the built-in mechanism itself. Nothing else
// in the routing depends on them, which is what makes them removable.
func pinnedMechanism(obj metav1.Object) (useUnified, pinned bool) {
	annotations := obj.GetAnnotations()

	if _, ok := annotations[v1alpha2.AnnUseBuiltInSnapshotter]; ok {
		return false, true
	}
	if _, ok := annotations[v1alpha2.AnnUseUnifiedSnapshotter]; ok {
		return true, true
	}

	return false, false
}

// capturedMechanism reports the mechanism that has already left its mark on an object, if any.
//
// A mechanism that captured an object keeps it. Without this the cluster default would silently
// reassign every snapshot taken before the default moved: on an upgrade that turns the unified
// mechanism on, a built-in snapshot sitting at phase Ready would be handed to the unified
// VirtualMachineSnapshot controller, which — seeing an empty domain capture phase — would plan the whole
// capture again against a live VirtualMachine.
//
// The built-in mark is any trace the built-in controller leaves, not only the CSI VolumeSnapshot or the
// Secret it produces: a built-in snapshot that failed or stalled before producing them is still the
// built-in controller's, and only that controller removes its cleanup finalizer on deletion.
func capturedMechanism(unified, builtIn bool) (useUnified, captured bool) {
	switch {
	case unified:
		return true, true
	case builtIn:
		return false, true
	default:
		return false, false
	}
}

// phasePastPending reports whether status.phase has moved beyond Pending.
//
// Without captureState such a phase can only come from the built-in controller: the unified one writes
// nothing but the Pending bootstrap before it applies captureState.
func phasePastPending[P ~string](phase, pending P) bool {
	return phase != "" && phase != pending
}

// pendingWithoutUnifiedFinalizer reports whether vms is in phase Pending without the unified cleanup
// finalizer, which only the built-in controller leaves.
//
// The unified controller adds FinalizerVMSnapshotCleanup before it writes the Pending bootstrap, and the
// built-in controller never adds it. A deleting object is excluded because the unified controller removes
// that finalizer at the end of its own deletion, so its absence no longer identifies the mechanism; such an
// object follows the cluster default.
func pendingWithoutUnifiedFinalizer(vms *v1alpha2.VirtualMachineSnapshot) bool {
	return vms.Status.Phase == v1alpha2.VirtualMachineSnapshotPhasePending &&
		vms.DeletionTimestamp == nil &&
		!controllerutil.ContainsFinalizer(vms, v1alpha2.FinalizerVMSnapshotCleanup)
}
