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

package vdsnapshot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/state-snapshotter/pkg/snapshotsdk"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/unified-snapshotter/internal/accessclaim"
	"github.com/deckhouse/virtualization-controller/pkg/controller/unified-snapshotter/internal/adapter"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	coordinationv1alpha1 "github.com/deckhouse/virtualization/api/coordination/v1alpha1"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdscondition"
)

// captureRefusedError is returned by claimAccess when the snapshot cannot be taken; the snapshot fails with its
// reason and message.
type captureRefusedError struct {
	reason  string
	message string
}

func (e *captureRefusedError) Error() string { return e.message }

// claimAccess creates the access claim of a snapshot taken on its own, for the virtual machine its disk is
// attached to. It returns no claim when the capture goes on without one, and a captureRefusedError when the
// snapshot cannot be taken.
//
// A disk attached to no virtual machine needs no access. The snapshot is marked instead, and fails if the disk
// gets attached during the capture. A disk attached to several virtual machines has no single guest to
// freeze, so it is captured without access unless consistency is required.
func (r *Reconciler) claimAccess(ctx context.Context, vds *v1alpha2.VirtualDiskSnapshot, vd *v1alpha2.VirtualDisk) (*coordinationv1alpha1.VirtualMachineAccessClaim, error) {
	vm, err := r.getAttachedVirtualMachine(ctx, vd)

	multiple := errors.Is(err, errMultipleAttachedVirtualMachines)
	if err != nil && !multiple {
		return nil, err
	}

	switch {
	case startedOnUnattachedDisk(vds):
		if vm != nil || multiple {
			return nil, &captureRefusedError{reason: string(vdscondition.VirtualMachineStateChanged), message: fmt.Sprintf(
				"The virtual disk %q was attached to a virtual machine while the snapshot was being taken. Take the snapshot again.", vd.Name)}
		}

		return nil, nil
	case multiple:
		logger.FromContext(ctx).Error("the disk is attached to several machines, so no single freeze covers it",
			slog.String("virtualDisk", vd.Name), logger.SlogErr(err))

		if vds.Spec.RequiredConsistency {
			return nil, &captureRefusedError{reason: string(vdscondition.PotentiallyInconsistent), message: fmt.Sprintf(
				"cannot take a consistent snapshot: %s", err)}
		}

		return nil, nil
	case vm == nil:
		return nil, r.markStartedOnUnattachedDisk(ctx, vds)
	}

	return accessclaim.Create(ctx, r.Client, r.APIReader, vds, vm, vds.Spec.RequiredConsistency)
}

// markStartedOnUnattachedDisk persists the mark before the capture starts, so later passes tell the snapshot
// from one started before the module update.
func (r *Reconciler) markStartedOnUnattachedDisk(ctx context.Context, vds *v1alpha2.VirtualDiskSnapshot) error {
	patch := client.MergeFrom(vds.DeepCopy())
	annotations.AddAnnotation(vds, annotations.AnnStartedOnUnattachedDisk, "true")

	return r.Client.Patch(ctx, vds, patch)
}

// startedBeforeUpdate reports a snapshot in progress started before the module update, which has no access
// to be captured under: a snapshot on its own past planning without a claim or the unattached-disk mark, or a
// child whose parent has no claim.
func (r *Reconciler) startedBeforeUpdate(ctx context.Context, vds *v1alpha2.VirtualDiskSnapshot, parent *metav1.OwnerReference, claim *coordinationv1alpha1.VirtualMachineAccessClaim, planning bool) (bool, error) {
	if parent == nil {
		return !planning && claim == nil && !startedOnUnattachedDisk(vds), nil
	}

	parentSnapshot := &v1alpha2.VirtualMachineSnapshot{ObjectMeta: metav1.ObjectMeta{UID: parent.UID}}
	parentClaim, err := accessclaim.Get(ctx, r.Client, r.APIReader, vds.Namespace, accessclaim.Name(parentSnapshot))

	return err == nil && parentClaim == nil, err
}

func startedOnUnattachedDisk(vds *v1alpha2.VirtualDiskSnapshot) bool {
	_, ok := vds.Annotations[annotations.AnnStartedOnUnattachedDisk]
	return ok
}

// waitForAccess holds the snapshot in Planning until the coordinator decides on the claim, and publishes
// why it waits (see accessclaim.Waiting).
func (r *Reconciler) waitForAccess(
	ctx context.Context,
	sdk snapshotsdk.CaptureSDK,
	a *adapter.VirtualDiskSnapshotAdapter,
	claim *coordinationv1alpha1.VirtualMachineAccessClaim,
) (ctrl.Result, error) {
	reason, message := accessclaim.Waiting(claim)
	if st := a.GetDomainCaptureState(); st.Reason != reason || st.Message != message {
		if err := sdk.DomainCaptureStatus(a).
			Phase(snapshotsdk.PhasePlanning).
			Reason(snapshotsdk.Reason(reason)).
			Message(message).
			Apply(ctx); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

// settleChildConsistency gives a terminal child the consistency of its parent once the parent is terminal
// (see accessclaim.ChildConsistency).
func (r *Reconciler) settleChildConsistency(ctx context.Context, vds *v1alpha2.VirtualDiskSnapshot, parent *metav1.OwnerReference) (ctrl.Result, error) {
	if vds.Status.Consistent != nil {
		return ctrl.Result{}, nil
	}

	consistent, settled, err := accessclaim.ChildConsistency(ctx, r.Client, vds, parent)
	if err != nil || !settled {
		// The watch on the parent brings its terminal phase here.
		return ctrl.Result{}, err
	}

	vds.Status.Consistent = ptr.To(consistent)

	return ctrl.Result{}, r.patchStatus(ctx, vds)
}
