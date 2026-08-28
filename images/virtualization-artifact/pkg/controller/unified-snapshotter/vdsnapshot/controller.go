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
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/state-snapshotter/pkg/snapshotsdk"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/unified-snapshotter/internal/adapter"
	"github.com/deckhouse/virtualization-controller/pkg/controller/unified-snapshotter/internal/annotation"
	"github.com/deckhouse/virtualization-controller/pkg/controller/unified-snapshotter/internal/statuspatch"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdscondition"
)

const (
	requeueAfter   = 2 * time.Second
	ControllerName = "virtualdisk-snapshot-controller"
)

// Reconciler drives VirtualDiskSnapshot capture through the state-snapshotter SDK.
type Reconciler struct {
	Client    client.Client
	APIReader client.Reader
	Freezer   *service.SnapshotService
	Log       *log.Logger
}

// SetupWithManager registers the reconciler, gated to objects annotated with
// v1alpha2.AnnUseUnifiedSnapshotter.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named(ControllerName).
		For(&v1alpha2.VirtualDiskSnapshot{}).
		WithLogConstructor(logger.NewConstructor(r.Log)).
		WithEventFilter(annotation.HasUnifiedSnapshotterAnnotation()).
		Complete(r)
}

func (r *Reconciler) sdk() snapshotsdk.CaptureSDK {
	return snapshotsdk.New(r.Client, r.APIReader, snapshotsdk.NewStorageFoundationProvider(r.Client))
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	vds := &v1alpha2.VirtualDiskSnapshot{}
	if err := r.Client.Get(ctx, req.NamespacedName, vds); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if vds.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}
	if _, ok := vds.Annotations[v1alpha2.AnnUseUnifiedSnapshotter]; !ok {
		return ctrl.Result{}, nil
	}

	if vds.Status.Phase == "" {
		vds.Status.Phase = v1alpha2.VirtualDiskSnapshotPhasePending
		if err := r.patchStatus(ctx, vds); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
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
		vds.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseReady
		return ctrl.Result{}, r.patchStatus(ctx, vds)
	}

	vd := &v1alpha2.VirtualDisk{}
	if err := r.Client.Get(ctx, types.NamespacedName{Namespace: vds.Namespace, Name: vds.Spec.VirtualDiskName}, vd); err != nil {
		if apierrors.IsNotFound(err) {
			if perr := sdk.DomainCaptureStatus(a).
				Phase(snapshotsdk.PhasePlanning).
				Message(fmt.Sprintf("source VirtualDisk %q not found; waiting", vds.Spec.VirtualDiskName)).
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
	if requestedSize, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; ok && vds.Status.PersistentVolumeClaimSize != requestedSize.String() {
		vds.Status.PersistentVolumeClaimSize = requestedSize.String()
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
		consistent, err := r.isConsistent(ctx, vd)
		switch {
		case err == nil:
			if consistent {
				vds.Status.Consistent = ptr.To(true)
				// Persist before the SDK calls below: they re-read this object from the API server into
				// vds, so an unpersisted local field would be silently dropped.
				if err := r.patchStatus(ctx, vds); err != nil {
					return ctrl.Result{}, err
				}
				break
			}
			if vds.Spec.RequiredConsistency {
				return r.failCapture(ctx, a, vds, string(vdscondition.PotentiallyInconsistent), fmt.Sprintf(
					"cannot take a consistent snapshot of virtual disk %q: the virtual machine it is attached to is running and its filesystem is not frozen",
					vds.Spec.VirtualDiskName))
			}
		case errors.Is(err, service.ErrUntrustedFilesystemFrozenCondition):
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
		vds.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseFailed
		return ctrl.Result{}, r.patchStatus(ctx, vds)
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
	// The capture is over, so the consistency question is settled: record the negative answer too. It is
	// only latched here, at the last possible moment, because the block above sets it the moment the disk is
	// observed frozen — writing false any earlier would pin a snapshot that simply had not been frozen yet.
	// Without this status.consistent stays absent, which reads as "unknown" and is indistinguishable from
	// "not computed".
	if vds.Status.Consistent == nil {
		vds.Status.Consistent = ptr.To(false)
	}
	vds.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseReady

	return ctrl.Result{}, r.patchStatus(ctx, vds)
}

func (r *Reconciler) failCapture(ctx context.Context, a *adapter.VirtualDiskSnapshotAdapter, vds *v1alpha2.VirtualDiskSnapshot, reason, message string) (ctrl.Result, error) {
	if err := r.sdk().DomainCaptureStatus(a).
		Phase(snapshotsdk.PhaseFailed).
		Reason(snapshotsdk.Reason(reason)).
		Message(message).
		Apply(ctx); err != nil {
		return ctrl.Result{}, err
	}

	vds.Status.Consistent = ptr.To(false)
	vds.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseFailed
	return ctrl.Result{}, r.patchStatus(ctx, vds)
}

func (r *Reconciler) isConsistent(ctx context.Context, vd *v1alpha2.VirtualDisk) (bool, error) {
	vm, err := r.getAttachedVirtualMachine(ctx, vd)
	if err != nil {
		return false, err
	}

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

func (r *Reconciler) getAttachedVirtualMachine(ctx context.Context, vd *v1alpha2.VirtualDisk) (*v1alpha2.VirtualMachine, error) {
	if len(vd.Status.AttachedToVirtualMachines) != 1 {
		// Not attached, or attached to several machines: there is no single freeze state to read.
		return nil, nil
	}

	vm := &v1alpha2.VirtualMachine{}
	err := r.Client.Get(ctx, types.NamespacedName{Namespace: vd.Namespace, Name: vd.Status.AttachedToVirtualMachines[0].Name}, vm)
	switch {
	case apierrors.IsNotFound(err):
		return nil, nil
	case err != nil:
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
