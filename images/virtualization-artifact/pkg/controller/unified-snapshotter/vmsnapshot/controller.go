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

// Package vmsnapshot drives VirtualMachineSnapshot capture through the state-snapshotter SDK
// (github.com/deckhouse/state-snapshotter/pkg/snapshotsdk). It is a manifest-only aggregator: its only
// child kind is VirtualDiskSnapshot (one per disk of the captured VirtualMachine), consistency is a
// single whole-VM guest-agent filesystem freeze held across all of them, and it carries no data leg of
// its own.
//
// The manifest leg (see manifest_targets.go) captures the same resource set as the old controller's
// Secret snapshot: the VirtualMachine itself, its VirtualMachineIPAddress, secondary-network
// VirtualMachineMACAddresses, the provisioner Secret, and hotplugged VirtualMachineBlockDeviceAttachments.
package vmsnapshot

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/deckhouse/deckhouse/pkg/log"
	storagev1alpha1 "github.com/deckhouse/state-snapshotter/api/storage/v1alpha1"
	"github.com/deckhouse/state-snapshotter/pkg/snapshotsdk"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/unified-snapshotter/internal/adapter"
	"github.com/deckhouse/virtualization-controller/pkg/controller/unified-snapshotter/internal/annotation"
	"github.com/deckhouse/virtualization-controller/pkg/controller/unified-snapshotter/internal/statuspatch"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmscondition"
)

const (
	requeueAfter   = 2 * time.Second
	ControllerName = "virtualmachine-snapshot-controller"

	childrenSettleDeadline  = 10 * time.Minute
	manifestTargetsDeadline = 2 * time.Minute
)

// Reconciler drives VirtualMachineSnapshot capture through the state-snapshotter SDK.
type Reconciler struct {
	Client    client.Client
	APIReader client.Reader
	// Freezer is the same *service.SnapshotService the built-in vmsnapshot/vdsnapshot controllers already
	// use for guest-agent filesystem freeze/unfreeze.
	Freezer *service.SnapshotService
	Log     *log.Logger
}

// SetupWithManager registers the reconciler, gated to objects annotated with
// v1alpha2.AnnUseUnifiedSnapshotter.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named(ControllerName).
		For(&v1alpha2.VirtualMachineSnapshot{}).
		WithLogConstructor(logger.NewConstructor(r.Log)).
		WithEventFilter(annotation.HasUnifiedSnapshotterAnnotation()).
		Complete(r)
}

func (r *Reconciler) sdk() snapshotsdk.CaptureSDK {
	return snapshotsdk.New(r.Client, r.APIReader, snapshotsdk.NewStorageFoundationProvider(r.Client))
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	vms := &v1alpha2.VirtualMachineSnapshot{}
	if err := r.Client.Get(ctx, req.NamespacedName, vms); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if vms.DeletionTimestamp != nil {
		return r.reconcileDeletion(ctx, vms)
	}
	if _, ok := vms.Annotations[v1alpha2.AnnUseUnifiedSnapshotter]; !ok {
		// Defensive: the manager-level predicate already filters this, but Reconcile may be invoked
		// directly (e.g. by an owned-object watch) so re-check before touching this object.
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(vms, v1alpha2.FinalizerVMSnapshotCleanup) {
		controllerutil.AddFinalizer(vms, v1alpha2.FinalizerVMSnapshotCleanup)
		if err := r.Client.Update(ctx, vms); err != nil {
			return ctrl.Result{}, err
		}
	}

	if vms.Status.Phase == "" {
		vms.Status.Phase = v1alpha2.VirtualMachineSnapshotPhasePending
		r.Log.Info("patch phase to VirtualMachineSnapshotPhasePending")
		if err := r.patchStatus(ctx, vms); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	a := &adapter.VirtualMachineSnapshotAdapter{VMS: vms}
	sdk := r.sdk()

	switch a.GetDomainCaptureState().Phase {
	case snapshotsdk.PhaseFinished:
		return r.reconcileCaptured(ctx, vms)
	case snapshotsdk.PhaseFailed:
		return r.reconcileCaptureFailed(ctx, vms)
	}

	vm := &v1alpha2.VirtualMachine{}
	if err := r.Client.Get(ctx, types.NamespacedName{Namespace: vms.Namespace, Name: vms.Spec.VirtualMachineName}, vm); err != nil {
		if apierrors.IsNotFound(err) {
			if perr := sdk.DomainCaptureStatus(a).
				Phase(snapshotsdk.PhasePlanning).
				Message(fmt.Sprintf("source VirtualMachine %q not found; waiting", vms.Spec.VirtualMachineName)).
				Apply(ctx); perr != nil {
				return ctrl.Result{}, perr
			}
			return ctrl.Result{RequeueAfter: requeueAfter}, nil
		}
		return ctrl.Result{}, err
	}
	if err := sdk.DomainCaptureStatus(a).Phase(snapshotsdk.PhasePlanning).Message("").Apply(ctx); err != nil {
		return ctrl.Result{}, err
	}
	if err := sdk.PublishSnapshotSource(ctx, a, snapshotsdk.SnapshotSource{
		APIVersion: v1alpha2.SchemeGroupVersion.String(),
		Kind:       v1alpha2.VirtualMachineKind,
		Name:       vm.Name,
		Namespace:  vm.Namespace,
		UID:        vm.UID,
	}); err != nil {
		return ctrl.Result{}, err
	}

	kvvmi, err := r.getKVVMI(ctx, vm)
	if err != nil {
		return ctrl.Result{}, err
	}
	if kvvmi != nil {
		if err := r.Freezer.SyncFSFreezeRequest(ctx, kvvmi); err != nil {
			return ctrl.Result{RequeueAfter: requeueAfter}, nil
		}
	}
	// Read the freeze state only after the sync, and never paper over an untrusted read. While a
	// freeze/unfreeze request is in flight IsFrozen reports ErrUntrustedFilesystemFrozenCondition; taking
	// that for "not frozen" sends us into the freeze block below, where CanFreeze reports false *because the
	// guest is already frozen* — indistinguishable there from "the agent cannot freeze it", and so it fails a
	// snapshot whose freeze in fact succeeded.
	frozen, err := r.Freezer.IsFrozen(kvvmi)
	if err != nil {
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	domainState := a.GetDomainCaptureState()
	planningNotFrozen := domainState.Phase != snapshotsdk.PhasePlanned &&
		domainState.Phase != snapshotsdk.PhaseFinished &&
		domainState.Phase != snapshotsdk.PhaseFailed
	if vms.Spec.RequiredConsistency && planningNotFrozen && !frozen {
		canFreeze, cErr := r.Freezer.CanFreeze(ctx, kvvmi)
		if cErr != nil {
			return ctrl.Result{}, cErr
		}
		if canFreeze {
			if err := r.Freezer.Freeze(ctx, kvvmi); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: requeueAfter}, nil
		}
		if kvvmi != nil && kvvmi.Status.Phase == virtv1.Running {
			return r.failCapture(ctx, a, vms, vm, kvvmi, string(vmscondition.PotentiallyInconsistent), fmt.Sprintf(
				"cannot take a consistent snapshot of virtual machine %q: the virtual machine agent is not ready and the virtual machine cannot be frozen", vm.Name))
		}
		// Not running (or no kvvmi): trivially consistent without a freeze.
	}

	children := r.planChildren(vms, vm)
	if err := sdk.EnsureChildren(ctx, a, children, nil); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{RequeueAfter: requeueAfter}, nil
		}
		return r.failCapture(ctx, a, vms, vm, kvvmi, storagev1alpha1.ReasonGraphPlanningFailed, err.Error())
	}
	targets, err := r.planManifestTargets(ctx, vms, vm)
	if err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{RequeueAfter: requeueAfter}, nil
		}

		if errors.Is(err, errManifestTargetNotReady) {
			if time.Since(vms.CreationTimestamp.Time) > manifestTargetsDeadline {
				return r.failCapture(ctx, a, vms, vm, kvvmi, storagev1alpha1.ReasonGraphPlanningFailed, fmt.Sprintf(
					"giving up after %s: %s", manifestTargetsDeadline, err))
			}
			if perr := sdk.DomainCaptureStatus(a).
				Phase(snapshotsdk.PhasePlanning).
				Message(fmt.Sprintf("waiting for a resource referenced for manifest capture: %s", err)).
				Apply(ctx); perr != nil {
				return ctrl.Result{}, perr
			}
			return ctrl.Result{RequeueAfter: requeueAfter}, nil
		}
		return r.failCapture(ctx, a, vms, vm, kvvmi, storagev1alpha1.ReasonGraphPlanningFailed, err.Error())
	}
	if err := sdk.EnsureManifestCapture(ctx, a, snapshotsdk.ManifestCaptureSpec{Targets: targets}); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{RequeueAfter: requeueAfter}, nil
		}
		return r.failCapture(ctx, a, vms, vm, kvvmi, storagev1alpha1.ReasonGraphPlanningFailed, err.Error())
	}
	if err := sdk.DomainCaptureStatus(a).Phase(snapshotsdk.PhasePlanned).Apply(ctx); err != nil {
		return ctrl.Result{}, err
	}

	outcome := snapshotsdk.CoreCaptureOutcome(a)
	switch outcome.Outcome {
	case snapshotsdk.CaptureOutcomeFailed:
		if !r.releaseFreeze(ctx, vms, vm, kvvmi) {
			return ctrl.Result{RequeueAfter: requeueAfter}, nil
		}
		vms.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseFailed
		r.Log.Info("patch VMS phase to VirtualMachineSnapshotPhaseFailed")
		return ctrl.Result{}, r.patchStatus(ctx, vms)
	case snapshotsdk.CaptureOutcomeCapturing:
		vms.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseInProgress
		r.Log.Info("patch VMS phase to VirtualMachineSnapshotPhaseInProgress")
		return ctrl.Result{RequeueAfter: requeueAfter}, r.patchStatus(ctx, vms)
	}

	// Own manifest leg captured; wait for every child VirtualDiskSnapshot to go terminal before
	// unfreezing (the whole point of one shared freeze across all disks). Gated on the core-computed
	// ChildrenSettled latch, not by inspecting each child's own legs via ChildrenCaptureStates: per the
	// SDK's own doc comment on ChildCaptureState, that per-child view is diagnostics-only and must not
	// gate a consistency action — it hangs forever on a child that fails with a domain-specific reason
	// outside the SDK's terminal-reason list. ChildrenSettled is the intended completeness signal
	// (true once every direct child is terminal, success or failure).

	cs := a.CoreCaptureState()
	childrenSettled := cs.ChildrenSettled != nil && *cs.ChildrenSettled
	if !childrenSettled {
		// A VolumeCaptureRequest has no guaranteed final state: storage-foundation keeps retrying a CSI
		// driver without a cap, so a child can stay non-terminal forever and ChildrenSettled never flips.
		// This is the only wait left that holds the guest filesystem frozen, so it needs an end.
		waited, pending, err := r.childrenWaitProgress(ctx, vms)
		if err != nil {
			return ctrl.Result{}, err
		}
		if waited > childrenSettleDeadline {
			return r.failCapture(ctx, a, vms, vm, kvvmi, string(vmscondition.VirtualMachineSnapshotFailed), fmt.Sprintf(
				"giving up after %s so the guest filesystem is not held frozen indefinitely: "+
					"VirtualDiskSnapshots %v have not finished capturing data",
				childrenSettleDeadline, pending))
		}

		vms.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseInProgress
		r.Log.Info("patch VMS phase to VirtualMachineSnapshotPhaseInProgress, children not settled")
		return ctrl.Result{RequeueAfter: requeueAfter}, r.patchStatus(ctx, vms)
	}

	if !r.releaseFreeze(ctx, vms, vm, kvvmi) {
		vms.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseInProgress
		return ctrl.Result{RequeueAfter: requeueAfter}, r.patchStatus(ctx, vms)
	}
	if err := sdk.DomainCaptureStatus(a).Phase(snapshotsdk.PhaseFinished).Apply(ctx); err != nil {
		return ctrl.Result{}, err
	}
	vms.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseReady

	consistent, err := r.childrenAreConsistent(ctx, vms)
	if err != nil {
		return ctrl.Result{}, err
	}
	if consistent {
		vms.Status.Consistent = ptr.To(true)
	}

	r.Log.Info("patch VMS phase to VirtualMachineSnapshotPhaseReady")
	return ctrl.Result{}, r.patchStatus(ctx, vms)
}

func (r *Reconciler) childrenWaitProgress(ctx context.Context, vms *v1alpha2.VirtualMachineSnapshot) (time.Duration, []string, error) {
	var (
		oldest  *metav1.Time
		pending []string
	)
	for _, ref := range vms.Status.ChildrenSnapshotRefs {
		if ref.Kind != v1alpha2.VirtualDiskSnapshotKind {
			continue
		}

		vds := &v1alpha2.VirtualDiskSnapshot{}
		if err := r.Client.Get(ctx, types.NamespacedName{Namespace: vms.Namespace, Name: ref.Name}, vds); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return 0, nil, err
		}
		if oldest == nil || vds.CreationTimestamp.Before(oldest) {
			oldest = vds.CreationTimestamp.DeepCopy()
		}
		switch vds.Status.Phase {
		case v1alpha2.VirtualDiskSnapshotPhaseReady, v1alpha2.VirtualDiskSnapshotPhaseFailed:
		default:
			pending = append(pending, vds.Name)
		}
	}
	sort.Strings(pending)
	if oldest == nil {
		return 0, pending, nil
	}
	return time.Since(oldest.Time), pending, nil
}

func (r *Reconciler) failCapture(
	ctx context.Context,
	a *adapter.VirtualMachineSnapshotAdapter,
	vms *v1alpha2.VirtualMachineSnapshot,
	vm *v1alpha2.VirtualMachine,
	kvvmi *virtv1.VirtualMachineInstance,
	reason, message string,
) (ctrl.Result, error) {
	if err := r.sdk().DomainCaptureStatus(a).
		Phase(snapshotsdk.PhaseFailed).
		Reason(snapshotsdk.Reason(reason)).
		Message(message).
		Apply(ctx); err != nil {
		return ctrl.Result{}, err
	}

	if !r.releaseFreeze(ctx, vms, vm, kvvmi) {
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	vms.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseFailed
	r.Log.Info("patch VMS phase to VirtualMachineSnapshotPhaseFailed", "reason", reason, "message", message)
	return ctrl.Result{}, r.patchStatus(ctx, vms)
}

func (r *Reconciler) reconcileCaptured(ctx context.Context, vms *v1alpha2.VirtualMachineSnapshot) (ctrl.Result, error) {
	if vms.Status.Phase == v1alpha2.VirtualMachineSnapshotPhaseReady {
		return ctrl.Result{}, nil
	}

	consistent, err := r.childrenAreConsistent(ctx, vms)
	if err != nil {
		return ctrl.Result{}, err
	}
	if consistent {
		vms.Status.Consistent = ptr.To(true)
	}

	vms.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseReady
	r.Log.Info("patch VMS phase to VirtualMachineSnapshotPhaseReady, capture already finished")
	return ctrl.Result{}, r.patchStatus(ctx, vms)
}

func (r *Reconciler) reconcileCaptureFailed(ctx context.Context, vms *v1alpha2.VirtualMachineSnapshot) (ctrl.Result, error) {
	if vms.Status.Phase == v1alpha2.VirtualMachineSnapshotPhaseFailed {
		return ctrl.Result{}, nil
	}

	vm := &v1alpha2.VirtualMachine{}
	err := r.Client.Get(ctx, types.NamespacedName{Namespace: vms.Namespace, Name: vms.Spec.VirtualMachineName}, vm)
	switch {
	case apierrors.IsNotFound(err):
		// No VirtualMachine: nothing holds a freeze.
	case err != nil:
		return ctrl.Result{}, err
	default:
		kvvmi, err := r.getKVVMI(ctx, vm)
		if err != nil {
			return ctrl.Result{}, err
		}
		if !r.releaseFreeze(ctx, vms, vm, kvvmi) {
			return ctrl.Result{RequeueAfter: requeueAfter}, nil
		}
	}

	vms.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseFailed
	r.Log.Info("patch VMS phase to VirtualMachineSnapshotPhaseFailed, capture already failed")
	return ctrl.Result{}, r.patchStatus(ctx, vms)
}

// reconcileDeletion unfreezes the guest filesystem if this snapshot's freeze is still held, then releases the finalizer.
func (r *Reconciler) reconcileDeletion(ctx context.Context, vms *v1alpha2.VirtualMachineSnapshot) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(vms, v1alpha2.FinalizerVMSnapshotCleanup) {
		return ctrl.Result{}, nil
	}

	vm := &v1alpha2.VirtualMachine{}
	err := r.Client.Get(ctx, types.NamespacedName{Namespace: vms.Namespace, Name: vms.Spec.VirtualMachineName}, vm)
	switch {
	case apierrors.IsNotFound(err):
		// No VM: nothing to unfreeze.
	case err != nil:
		return ctrl.Result{}, err
	default:
		kvvmi, err := r.getKVVMI(ctx, vm)
		if err != nil {
			return ctrl.Result{}, err
		}

		if !r.releaseFreeze(ctx, vms, vm, kvvmi) {
			return ctrl.Result{RequeueAfter: requeueAfter}, nil
		}
	}

	controllerutil.RemoveFinalizer(vms, v1alpha2.FinalizerVMSnapshotCleanup)
	return ctrl.Result{}, r.Client.Update(ctx, vms)
}

func (r *Reconciler) releaseFreeze(ctx context.Context, vms *v1alpha2.VirtualMachineSnapshot, vm *v1alpha2.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance) bool {
	if kvvmi == nil {
		return true
	}

	if err := r.Freezer.SyncFSFreezeRequest(ctx, kvvmi); err != nil {
		r.Log.Debug("failed to sync the guest filesystem freeze request, will retry", "err", err.Error())
		return false
	}

	frozen, err := r.Freezer.IsFrozen(kvvmi)
	if err != nil {
		r.Log.Debug("failed to read the guest filesystem freeze state, will retry", "err", err.Error())
		return false
	}
	if !frozen {
		return true
	}

	canUnfreeze, err := r.Freezer.CanUnfreezeWithVirtualMachineSnapshotTree(ctx, vms, vm, kvvmi)
	switch {
	case errors.Is(err, service.ErrUntrustedFilesystemFrozenCondition):
		return false
	case err != nil:
		r.Log.Error("failed to check whether the guest filesystem freeze may be released, will retry", "err", err.Error())
		return false
	}
	if !canUnfreeze {
		// Another in-flight snapshot of the same VirtualMachine still holds the freeze and will release
		// it itself. Nothing left for this snapshot to wait on.
		return true
	}

	if err := r.Freezer.Unfreeze(ctx, kvvmi); err != nil {
		r.Log.Debug("failed to request guest filesystem unfreeze, will retry", "err", err.Error())
	}

	return false
}

// childrenAreConsistent aggregates the consistency each child VirtualDiskSnapshot resolved for itself:
// the whole-VM snapshot is consistent only when every captured disk is.
func (r *Reconciler) childrenAreConsistent(ctx context.Context, vms *v1alpha2.VirtualMachineSnapshot) (bool, error) {
	for _, ref := range vms.Status.ChildrenSnapshotRefs {
		if ref.Kind != v1alpha2.VirtualDiskSnapshotKind {
			continue
		}

		vds := &v1alpha2.VirtualDiskSnapshot{}
		if err := r.Client.Get(ctx, types.NamespacedName{Namespace: vms.Namespace, Name: ref.Name}, vds); err != nil {
			if apierrors.IsNotFound(err) {
				// A child that no longer exists cannot vouch for its disk, so consistency stays unknown
				return false, nil
			}
			return false, err
		}

		if vds.Status.Consistent == nil || !*vds.Status.Consistent {
			return false, nil
		}
	}

	return true, nil
}

// vmsOwnedStatus lists exactly the VirtualMachineSnapshotStatus fields this controller ever sets. Every
// other status field (captureState.commonController, boundSnapshotContentName, conditions,
// childrenSnapshotRefs, ...) is owned by the core/SDK and deliberately absent here, so patchStatus's
// merge patch never touches them — see internal/statuspatch.
type vmsOwnedStatus struct {
	Phase                    v1alpha2.VirtualMachineSnapshotPhase `json:"phase,omitempty"`
	Consistent               *bool                                `json:"consistent,omitempty"`
	VirtualDiskSnapshotNames []string                             `json:"virtualDiskSnapshotNames,omitempty"`
}

func (r *Reconciler) patchStatus(ctx context.Context, vms *v1alpha2.VirtualMachineSnapshot) error {
	patch, err := statuspatch.For(v1alpha2.SchemeGroupVersion.WithKind(v1alpha2.VirtualMachineSnapshotKind), vmsOwnedStatus{
		Phase:                    vms.Status.Phase,
		Consistent:               vms.Status.Consistent,
		VirtualDiskSnapshotNames: vdSnapshotNames(vms.Status.ChildrenSnapshotRefs),
	})
	if err != nil {
		return err
	}
	return r.Client.Status().Patch(ctx, vms, patch)
}

func vdSnapshotNames(refs []v1alpha2.UnifiedSnapshotterChildRef) []string {
	names := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref.Kind == v1alpha2.VirtualDiskSnapshotKind {
			names = append(names, ref.Name)
		}
	}
	sort.Strings(names)
	return names
}

// planChildren builds the desired VirtualDiskSnapshot set: one per disk device attached to vm, deriving
// each a deterministic name so EnsureChildren's create-or-adopt stays idempotent across reconciles. Each
// child carries AnnUseUnifiedSnapshotter itself, so it is in turn driven by this same SDK-based
// controller family (vdsnapshot), never by the built-in one.
func (r *Reconciler) planChildren(vms *v1alpha2.VirtualMachineSnapshot, vm *v1alpha2.VirtualMachine) []snapshotsdk.ChildSpec {
	specs := make([]snapshotsdk.ChildSpec, 0, len(vm.Status.BlockDeviceRefs))
	for _, bdr := range vm.Status.BlockDeviceRefs {
		if bdr.Kind != v1alpha2.DiskDevice {
			continue
		}
		child := &v1alpha2.VirtualDiskSnapshot{
			ObjectMeta: childObjectMeta(vms, bdr.Name),
			Spec: v1alpha2.VirtualDiskSnapshotSpec{
				VirtualDiskName:     bdr.Name,
				RequiredConsistency: vms.Spec.RequiredConsistency,
			},
		}
		specs = append(specs, snapshotsdk.ChildSpec{Object: child})
	}
	return specs
}

func childObjectMeta(vms *v1alpha2.VirtualMachineSnapshot, diskName string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      fmt.Sprintf("%s-%s", diskName, vms.UID),
		Namespace: vms.Namespace,
		Annotations: map[string]string{
			v1alpha2.AnnUseUnifiedSnapshotter: "",
		},
	}
}

func (r *Reconciler) getKVVMI(ctx context.Context, vm *v1alpha2.VirtualMachine) (*virtv1.VirtualMachineInstance, error) {
	// APIReader (uncached, direct) is deliberate here: r.Client is the manager's cached client, and Get()
	// on a GVK it hasn't seen yet lazily starts a cluster-wide List+Watch informer for that whole type —
	// we only ever need this one VMI, not a standing cache of every VirtualMachineInstance in the cluster.
	kvvmi := &virtv1.VirtualMachineInstance{}
	if err := r.APIReader.Get(ctx, types.NamespacedName{Namespace: vm.Namespace, Name: vm.Name}, kvvmi); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return kvvmi, nil
}
