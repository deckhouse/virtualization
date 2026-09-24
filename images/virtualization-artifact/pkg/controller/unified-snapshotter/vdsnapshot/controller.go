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

// Package vdsnapshot drives VirtualDiskSnapshot capture through the state-snapshotter SDK
// (github.com/deckhouse/state-snapshotter/pkg/snapshotsdk). It is a data-bearing leaf: its data leg is
// the disk's backing PVC, captured via EnsureVolumeCapture (a storage-foundation VolumeCaptureRequest)
// instead of a directly-created CSI VolumeSnapshot.
//
// Scope note (limited PoC): this controller never freezes/unfreezes the guest filesystem itself.
// VirtualDiskSnapshot objects in this PoC are always planned as children of a VirtualMachineSnapshot
// (see the vmsnapshot package), which holds one filesystem freeze across all of a VM's disks; a disk
// snapshot taken standalone (not through a VirtualMachineSnapshot) is out of scope here.
package vdsnapshot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/state-snapshotter/pkg/snapshotsdk"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	commonvd "github.com/deckhouse/virtualization-controller/pkg/common/vd"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/unified-snapshotter/internal/adapter"
	"github.com/deckhouse/virtualization-controller/pkg/controller/unified-snapshotter/internal/annotation"
	"github.com/deckhouse/virtualization-controller/pkg/controller/unified-snapshotter/internal/freezelog"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization-controller/pkg/unifiedsnapshotter/statuspatch"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdscondition"
)

const (
	requeueAfter   = 2 * time.Second
	ControllerName = "virtualdisk-snapshot-controller"

	// reasonInvalidSource is the terminal domain reason for a snapshot whose spec does not resolve to a
	// capturable source object.
	reasonInvalidSource = "InvalidSource"
)

// Reconciler drives VirtualDiskSnapshot capture through the state-snapshotter SDK.
type Reconciler struct {
	Client    client.Client
	APIReader client.Reader
	Freezer   *service.SnapshotService
	Log       *log.Logger
}

// SetupWithManager registers the reconciler, gated to the objects the unified mechanism owns — see
// annotation.DrivenByUnified.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named(ControllerName).
		For(&v1alpha2.VirtualDiskSnapshot{}).
		WithLogConstructor(logger.NewConstructor(r.Log)).
		WithEventFilter(annotation.ShouldHandle()).
		Complete(r)
}

func (r *Reconciler) sdk() snapshotsdk.CaptureSDK {
	return snapshotsdk.New(r.Client, r.APIReader, snapshotsdk.NewStorageFoundationProvider(r.Client))
}

// Reconcile drives one VirtualDiskSnapshot, stopping quietly if it is deleted while the work is in
// flight. See the VirtualMachineSnapshot controller's Reconcile for why the first read's guard is not
// enough on its own.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	result, err := r.reconcile(ctx, req)
	if object.IsGone(err, v1alpha2.SchemeGroupVersion.WithResource(v1alpha2.VirtualDiskSnapshotResource).GroupResource(), req.Name) {
		logger.FromContext(ctx).Debug("the snapshot was deleted while it was being reconciled; nothing left to do")
		return ctrl.Result{}, nil
	}

	return result, err
}

func (r *Reconciler) reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	vds := &v1alpha2.VirtualDiskSnapshot{}
	if err := r.Client.Get(ctx, req.NamespacedName, vds); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if vds.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}
	driven, err := annotation.DrivenByUnifiedVirtualDiskSnapshot(ctx, r.APIReader, vds)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !driven {
		return ctrl.Result{}, nil
	}
	if vds.IsImport() {
		return r.reconcileImport(ctx, vds)
	}
	if vds.Status.Phase == "" {
		vds.Status.Phase = v1alpha2.VirtualDiskSnapshotPhasePending
		if err := r.patchStatus(ctx, vds); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: time.Second}, nil
	}

	a := &adapter.VirtualDiskSnapshotAdapter{VDS: vds}
	sdk := r.sdk()
	domainPhase := a.GetDomainCaptureState().Phase
	planning := domainPhase != snapshotsdk.PhasePlanned && domainPhase != snapshotsdk.PhaseFinished

	switch {
	case vds.Status.Phase == v1alpha2.VirtualDiskSnapshotPhaseReady,
		vds.Status.Phase == v1alpha2.VirtualDiskSnapshotPhaseFailed:
		return ctrl.Result{}, nil
	case domainPhase == snapshotsdk.PhaseFinished:
		return r.finishAsReady(ctx, vds)
	}

	// An unresolvable source is failed only here, AFTER the switch above: failing it earlier pre-empted the
	// Pending bootstrap, so an object the cluster default hands to this controller never got a phase, and
	// it bypassed finishAsFailed. Mirrors the same ordering in the vmsnapshot controller.
	vdName := vds.SourceVirtualDiskName()
	if vdName == "" {
		return r.failCapture(ctx, a, vds, reasonInvalidSource, fmt.Sprintf(
			"spec.sourceRef must reference a %s %s", v1alpha2.SchemeGroupVersion.String(), v1alpha2.VirtualDiskKind))
	}

	vd := &v1alpha2.VirtualDisk{}
	if err := r.Client.Get(ctx, types.NamespacedName{Namespace: vds.Namespace, Name: vdName}, vd); err != nil {
		if apierrors.IsNotFound(err) {
			// Waited on indefinitely, mirroring the VirtualMachine wait in the vmsnapshot controller: nothing
			// is frozen at this point, so the wait holds nothing hostage, and it absorbs the apply ordering
			// race of a snapshot and its disk landing in one manifest set. Per the SDK a non-terminal
			// "waiting for X" stays in Planning and reports itself through DomainCaptureStatus rather than
			// failing; the reason is what makes it machine-readable, since conditions here are core-owned.
			if perr := sdk.DomainCaptureStatus(a).
				Phase(snapshotsdk.PhasePlanning).
				Reason(snapshotsdk.Reason(vdscondition.WaitingForTheVirtualDisk)).
				Message(fmt.Sprintf("The VirtualDisk %q does not exist. Waiting for it to appear; snapshotting will continue on its own.", vdName)).
				Apply(ctx); perr != nil {
				return ctrl.Result{}, perr
			}
			return ctrl.Result{RequeueAfter: requeueAfter}, nil
		}
		return ctrl.Result{}, err
	}

	pvcName := vd.Status.Target.PersistentVolumeClaim
	if pvcName == "" {
		if perr := sdk.DomainCaptureStatus(a).
			Phase(snapshotsdk.PhasePlanning).
			Message("waiting for the virtual disk to provision its backing PersistentVolumeClaim").
			Apply(ctx); perr != nil {
			return ctrl.Result{}, perr
		}
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}
	// APIReader (uncached, direct): r.Client is the manager's cached client, and Get() on a GVK it
	// hasn't seen yet lazily starts a cluster-wide List+Watch informer for that whole type — we only
	// ever need this one PVC, not a standing cache of every PersistentVolumeClaim in the cluster.
	pvc := &corev1.PersistentVolumeClaim{}
	if err := r.APIReader.Get(ctx, types.NamespacedName{Namespace: vds.Namespace, Name: pvcName}, pvc); err != nil {
		if apierrors.IsNotFound(err) {
			if perr := sdk.DomainCaptureStatus(a).
				Phase(snapshotsdk.PhasePlanning).
				Message(fmt.Sprintf("backing PersistentVolumeClaim %q not found; waiting", pvcName)).
				Apply(ctx); perr != nil {
				return ctrl.Result{}, perr
			}
			return ctrl.Result{RequeueAfter: requeueAfter}, nil
		}
		return ctrl.Result{}, err
	}
	if pvc.Status.Phase != corev1.ClaimBound {
		if err := sdk.DomainCaptureStatus(a).
			Phase(snapshotsdk.PhasePlanning).
			Message(fmt.Sprintf("backing PersistentVolumeClaim %q is not bound yet (%s); waiting", pvcName, pvc.Status.Phase)).
			Apply(ctx); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	if err := sdk.DomainCaptureStatus(a).Phase(snapshotsdk.PhasePlanning).Message("").Apply(ctx); err != nil {
		return ctrl.Result{}, err
	}

	var mirrored bool
	if sc := pvc.Spec.StorageClassName; sc != nil && *sc != "" && vds.Status.StorageClassName != *sc {
		vds.Status.StorageClassName = *sc
		mirrored = true
	}
	// Mirror what the disk declares, not what its claim happens to request: the two diverge once the
	// claim was grown past the declaration (a restore rounded up to the driver floor, a clone sized to
	// the source's capacity), and the restore path fails a disk asking for less than this value.
	if requestedSize := commonvd.RequestedSize(vd, pvc); requestedSize != "" && vds.Status.PersistentVolumeClaimSize != requestedSize {
		vds.Status.PersistentVolumeClaimSize = requestedSize
		mirrored = true
	}
	if mirrored {
		// Persist right away so it's not silently dropped
		if err := r.patchStatus(ctx, vds); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := sdk.PublishSnapshotSource(ctx, a, snapshotsdk.SnapshotSource{
		APIVersion: v1alpha2.SchemeGroupVersion.String(),
		Kind:       v1alpha2.VirtualDiskKind,
		Name:       vd.Name,
		Namespace:  vd.Namespace,
		UID:        vd.UID,
	}); err != nil {
		return ctrl.Result{}, err
	}

	if planning && vds.Status.Consistent == nil {
		// The freeze this disk is captured under belongs to the parent VirtualMachineSnapshot, and the
		// verdict below is reached once and never revisited. Record what was actually observed — the
		// machine, the instance and its freeze state — so a disk that reports "running and not frozen"
		// can be lined up against the parent's own freeze records and the sibling disks' verdicts.
		consistent, err := r.isConsistent(ctx, vd)
		observed := r.observedFreezeState(ctx, vd)
		log := freezelog.For(ctx).With(slog.String("virtualDisk", vd.Name))

		switch {
		case err == nil:
			if consistent {
				log.Info("the disk may be captured consistently",
					freezelog.Attrs(observed, slog.Bool("requiredConsistency", vds.Spec.RequiredConsistency))...)
				vds.Status.Consistent = ptr.To(true)
				// Persist before the SDK calls below: they re-read this object from the API server into
				// vds, so an unpersisted local field would be silently dropped.
				if err := r.patchStatus(ctx, vds); err != nil {
					return ctrl.Result{}, err
				}
				break
			}
			if vds.Spec.RequiredConsistency {
				log.Error("the guest filesystem is not frozen; failing the capture", freezelog.State(observed)...)
				return r.failCapture(ctx, a, vds, string(vdscondition.PotentiallyInconsistent), fmt.Sprintf(
					"cannot take a consistent snapshot of virtual disk %q: the virtual machine it is attached to is running and its filesystem is not frozen",
					vds.SourceVirtualDiskName()))
			}
			log.Info("the guest filesystem is not frozen; capturing anyway, no consistency was required",
				freezelog.State(observed)...)
		case errors.Is(err, errMultipleAttachedVirtualMachines):
			// A disk attached to several running machines cannot be frozen unambiguously, so a consistency
			// mandate cannot be honored.
			log.Error("the disk is attached to several machines, so no single freeze covers it",
				logger.SlogErr(err), slog.Bool("requiredConsistency", vds.Spec.RequiredConsistency))
			if vds.Spec.RequiredConsistency {
				return r.failCapture(ctx, a, vds, string(vdscondition.PotentiallyInconsistent), fmt.Sprintf(
					"cannot take a consistent snapshot: %s", err))
			}
		case errors.Is(err, service.ErrUntrustedFilesystemFrozenCondition):
			log.Debug("a freeze request is still in flight; waiting before deciding consistency",
				freezelog.State(observed)...)
			return ctrl.Result{RequeueAfter: requeueAfter}, nil
		default:
			return ctrl.Result{}, err
		}
	}

	if err := sdk.EnsureVolumeCapture(ctx, a, snapshotsdk.VolumeCaptureSpec{
		DataRef: &snapshotsdk.Target{
			UID:        string(pvc.UID),
			APIVersion: "v1",
			Kind:       "PersistentVolumeClaim",
			Name:       pvc.Name,
			Namespace:  pvc.Namespace,
		},
	}); err != nil {
		return ctrl.Result{}, err
	}
	if err := sdk.EnsureManifestCapture(ctx, a, snapshotsdk.ManifestCaptureSpec{
		Targets: []snapshotsdk.ManifestTarget{{
			APIVersion: v1alpha2.SchemeGroupVersion.String(),
			Kind:       v1alpha2.VirtualDiskKind,
			Name:       vd.Name,
		}},
	}); err != nil {
		return ctrl.Result{}, err
	}

	if err := sdk.DomainCaptureStatus(a).Phase(snapshotsdk.PhasePlanned).Apply(ctx); err != nil {
		return ctrl.Result{}, err
	}

	// Barrier 2 (Finished): switch on the SDK-derived capture outcome. The snapshotter core flips latches in the
	// Status.CommonController as capture progresses.
	// A disk is a data-leaf: it confirms consistency immediately once all its
	// declared legs (manifest + data) are captured — PoC don't implement VM freeze/unfreeze for a
	// single volume.
	switch outcome := snapshotsdk.CoreCaptureOutcome(a); outcome.Outcome {
	case snapshotsdk.CaptureOutcomeFailed:
		// A failed manifest/data leg is declared terminal by the CORE, not by us: the core marks the
		// bound SnapshotContent itself terminal (e.g. reason VolumeCaptureFailed) and that is what
		// CoreCaptureOutcome/ReadyStatus read here. We deliberately do not try to push our own
		// captureState.domainSpecificController.phase to Failed to match it — the SDK has no verb for
		// that (only the domain's own Planning/Planned/Finished progression), and the core's terminal
		// state is already durable and final. There is nothing left for us to drive, so we just record
		// our own status.phase field for observability and stop — requeuing would only poll a decision
		// that has already been made.
		return r.finishAsFailed(ctx, vds)
	case snapshotsdk.CaptureOutcomeCapturing:
		// Capturing: wait for the core to finish. The status watch wakes us on each leg latch flip;
		// use requeue as a fallback in case a signal is missed.
		vds.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseInProgress
		return ctrl.Result{RequeueAfter: requeueAfter}, r.patchStatus(ctx, vds)
	}

	if vds.Status.Data == nil {
		// The core mirrors the bound SnapshotContent's data binding onto status.data, and does it on its
		// own schedule. We don't watch SnapshotContent, so nothing else will wake this object up once that
		// lands — requeue explicitly instead of settling into Ready without a data binding to restore from.
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}
	if err := sdk.DomainCaptureStatus(a).Phase(snapshotsdk.PhaseFinished).Apply(ctx); err != nil {
		return ctrl.Result{}, err
	}
	return r.finishAsReady(ctx, vds)
}

// settleConsistency records the negative answer to the consistency question once the capture reaches a
// terminal state. It is latched this late on purpose: the planning block sets it to true the moment the
// disk is observed frozen, so writing false any earlier would pin a snapshot that simply had not been
// frozen yet. Leaving it absent is not an option either — a terminal VirtualDiskSnapshot never
// reconciles again, and childrenAreConsistent reads a missing value as inconsistent, which would sink
// the whole VirtualMachineSnapshot.
func settleConsistency(vds *v1alpha2.VirtualDiskSnapshot) {
	if vds.Status.Consistent == nil {
		vds.Status.Consistent = ptr.To(false)
	}
}

// reconcileImport reports the progress the core is making on an import-mode snapshot and touches nothing
// else. See the vmsnapshot controller's reconcileImport for why there is nothing here to drive, and why
// status.consistent is left alone.
//
// status.storageClassName and status.persistentVolumeClaimSize are left alone for the same reason: they
// mirror the disk as it was at capture time, and the disk that was captured is described by the manifests
// in the archive, not by anything reachable from here. They exist to default a NEW VirtualDisk cloned
// from this snapshot; a restore does not need them, because the archive carries the VirtualDisk with its
// own size and storage class.
func (r *Reconciler) reconcileImport(ctx context.Context, vds *v1alpha2.VirtualDiskSnapshot) (ctrl.Result, error) {
	phase := v1alpha2.VirtualDiskSnapshotPhasePending
	switch {
	case meta.IsStatusConditionTrue(vds.Status.Conditions, v1alpha2.UnifiedSnapshotterConditionReady):
		phase = v1alpha2.VirtualDiskSnapshotPhaseReady
	case vds.Status.BoundSnapshotContentName != "":
		phase = v1alpha2.VirtualDiskSnapshotPhaseInProgress
	}

	if vds.Status.Phase == phase {
		return ctrl.Result{}, nil
	}

	vds.Status.Phase = phase
	return ctrl.Result{}, r.patchStatus(ctx, vds)
}

func (r *Reconciler) finishAsReady(ctx context.Context, vds *v1alpha2.VirtualDiskSnapshot) (ctrl.Result, error) {
	settleConsistency(vds)
	vds.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseReady

	logger.FromContext(ctx).Info("patch VDS phase to VirtualDiskSnapshotPhaseReady",
		slog.Bool("consistent", vds.Status.Consistent != nil && *vds.Status.Consistent))
	return ctrl.Result{}, r.patchStatus(ctx, vds)
}

func (r *Reconciler) finishAsFailed(ctx context.Context, vds *v1alpha2.VirtualDiskSnapshot) (ctrl.Result, error) {
	settleConsistency(vds)
	vds.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseFailed

	logger.FromContext(ctx).Info("patch VDS phase to VirtualDiskSnapshotPhaseFailed")
	return ctrl.Result{}, r.patchStatus(ctx, vds)
}

func (r *Reconciler) failCapture(ctx context.Context, a *adapter.VirtualDiskSnapshotAdapter, vds *v1alpha2.VirtualDiskSnapshot, reason, message string) (ctrl.Result, error) {
	logger.FromContext(ctx).Error("failing the disk capture",
		slog.String("reason", reason), slog.String("message", message))

	if err := r.sdk().DomainCaptureStatus(a).
		Phase(snapshotsdk.PhaseFailed).
		Reason(snapshotsdk.Reason(reason)).
		Message(message).
		Apply(ctx); err != nil {
		return ctrl.Result{}, err
	}

	return r.finishAsFailed(ctx, vds)
}

// observedFreezeState re-reads the VirtualMachineInstance behind the disk purely so the log can say what
// the consistency verdict was based on. Failures are swallowed: this must never change the outcome of a
// capture, and a nil instance is a truthful record of "nothing could be read".
func (r *Reconciler) observedFreezeState(ctx context.Context, vd *v1alpha2.VirtualDisk) *virtv1.VirtualMachineInstance {
	vm, err := r.getAttachedVirtualMachine(ctx, vd)
	if err != nil || vm == nil {
		return nil
	}

	kvvmi, err := r.getKVVMI(ctx, vm)
	if err != nil {
		return nil
	}
	return kvvmi
}

func (r *Reconciler) isConsistent(ctx context.Context, vd *v1alpha2.VirtualDisk) (bool, error) {
	vm, err := r.getAttachedVirtualMachine(ctx, vd)
	if err != nil {
		return false, err
	}

	// Nothing to freeze: no VirtualMachine holds the disk, or the one that does is stopped.
	if vm == nil || vm.Status.Phase == v1alpha2.MachineStopped {
		return true, nil
	}

	kvvmi, err := r.getKVVMI(ctx, vm)
	if err != nil {
		return false, err
	}
	if kvvmi == nil || kvvmi.Status.Phase != virtv1.Running {
		return true, nil
	}

	frozen, err := r.Freezer.IsFrozen(kvvmi)
	if err != nil {
		return false, err
	}

	return frozen, nil
}

var errMultipleAttachedVirtualMachines = errors.New("attached to multiple virtual machines")

// getAttachedVirtualMachine returns the one VirtualMachine the virtual disk is attached to.
//
// A nil VirtualMachine means there is nothing to freeze: the disk is attached to none, or the one it
// names is already gone. Several attached VirtualMachines are reported as
// errMultipleAttachedVirtualMachines instead — which one to freeze is ambiguous, so the caller decides
// what that means for the snapshot rather than mistaking it for a disk nobody uses.
func (r *Reconciler) getAttachedVirtualMachine(ctx context.Context, vd *v1alpha2.VirtualDisk) (*v1alpha2.VirtualMachine, error) {
	attached := vd.Status.AttachedToVirtualMachines

	if len(attached) == 0 {
		return nil, nil
	}

	if len(attached) > 1 {
		names := make([]string, 0, len(attached))
		for _, vm := range attached {
			names = append(names, vm.Name)
		}
		return nil, fmt.Errorf("the virtual disk %q is %w: %s", vd.Name, errMultipleAttachedVirtualMachines, strings.Join(names, ", "))
	}

	vm := &v1alpha2.VirtualMachine{}
	err := r.Client.Get(ctx, types.NamespacedName{Namespace: vd.Namespace, Name: attached[0].Name}, vm)
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return vm, nil
}

func (r *Reconciler) getKVVMI(ctx context.Context, vm *v1alpha2.VirtualMachine) (*virtv1.VirtualMachineInstance, error) {
	kvvmi := &virtv1.VirtualMachineInstance{}
	if err := r.APIReader.Get(ctx, types.NamespacedName{Namespace: vm.Namespace, Name: vm.Name}, kvvmi); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return kvvmi, nil
}

// vdsOwnedStatus lists exactly the VirtualDiskSnapshotStatus fields this controller ever sets. Every
// other status field (captureState.commonController, boundSnapshotContentName, conditions, ...) is
// owned by the core/SDK and deliberately absent here, so patchStatus's merge patch never touches them —
// see internal/statuspatch.
type vdsOwnedStatus struct {
	Phase                     v1alpha2.VirtualDiskSnapshotPhase `json:"phase,omitempty"`
	Consistent                *bool                             `json:"consistent,omitempty"`
	StorageClassName          string                            `json:"storageClassName,omitempty"`
	PersistentVolumeClaimSize string                            `json:"persistentVolumeClaimSize,omitempty"`
}

func (r *Reconciler) patchStatus(ctx context.Context, vds *v1alpha2.VirtualDiskSnapshot) error {
	patch, err := statuspatch.For(v1alpha2.SchemeGroupVersion.WithKind(v1alpha2.VirtualDiskSnapshotKind), vdsOwnedStatus{
		Phase:                     vds.Status.Phase,
		Consistent:                vds.Status.Consistent,
		StorageClassName:          vds.Status.StorageClassName,
		PersistentVolumeClaimSize: vds.Status.PersistentVolumeClaimSize,
	})
	if err != nil {
		return err
	}
	return r.Client.Status().Patch(ctx, vds, patch)
}
