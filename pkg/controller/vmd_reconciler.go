package controller

import (
	"context"
	"fmt"
	virtv2 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type VMDReconciler struct {
	client   client.Client
	recorder record.EventRecorder
	scheme   *runtime.Scheme
	log      logr.Logger
}

func (r *VMDReconciler) Reconcile(ctx context.Context, req reconcile.Request) (res reconcile.Result, err error) {
	log := r.log.WithValues("VirtualMachineDisk", req.NamespacedName)

	log.Info(fmt.Sprintf("sync phase begin"))
	syncState, syncErr := r.sync(ctx, log, req)
	log.Info(fmt.Sprintf("sync phase end"))

	log.Info(fmt.Sprintf("update status phase begin"))
	updateStatusState, updateStatusErr := r.updateStatus(ctx, log, req, syncState.PhaseResult)
	log.Info(fmt.Sprintf("update status phase end"))

	if syncErr != nil {
		err = syncErr
	} else if updateStatusErr != nil {
		err = updateStatusErr
	}

	if syncState.Result != nil {
		res = *syncState.Result
	} else if updateStatusState.Result != nil {
		res = *updateStatusState.Result
	}

	return res, err
}

func (r *VMDReconciler) sync(ctx context.Context, log logr.Logger, req reconcile.Request) (VMDReconcilerSyncState, error) {
	syncState, err := r.doSync(ctx, log, req)
	if err == nil {
		err = r.doSyncApplyMutated(ctx, log, syncState)
	}
	return syncState, err
}

func (r *VMDReconciler) newState(ctx context.Context, log logr.Logger, req reconcile.Request) (*VMDReconcilerState, error) {
	state := &VMDReconcilerState{}

	vmd, err := FetchObject(ctx, req.NamespacedName, r.client, &virtv2.VirtualMachineDisk{})
	if err != nil {
		return nil, fmt.Errorf("unable to get %q: %w", req.NamespacedName, err)
	}
	if vmd == nil {
		log.Info(fmt.Sprintf("Reconcile observe absent VMD: %s, it may be deleted", req.String()))
		return nil, nil
	}
	state.VMD = vmd
	state.VMDMutated = vmd.DeepCopy()

	state.DV, err = FetchObject(ctx, req.NamespacedName, r.client, &cdiv1.DataVolume{})
	if err != nil {
		return nil, fmt.Errorf("unable to get %q: %w", req.NamespacedName, err)
	}

	return state, nil
}

func (r *VMDReconciler) doSync(ctx context.Context, log logr.Logger, req reconcile.Request) (VMDReconcilerSyncState, error) {
	syncState := VMDReconcilerSyncState{}
	if cs, err := r.newState(ctx, log, req); err != nil {
		return VMDReconcilerSyncState{}, err
	} else if cs == nil {
		log.Info(fmt.Sprintf("Reconcile observe absent VMD: %s, it may be deleted", req.String()))
		return VMDReconcilerSyncState{}, nil
	} else {
		syncState.VMDReconcilerState = *cs
	}

	if syncState.DV == nil {
		var err error

		// TODO: How to set custom PVC name using DataVolume spec?
		// DataVolume named after VirtualMachineDisk (?)
		dv := NewDVFromVirtualMachineDisk(req.Namespace, req.Name, syncState.VMD)

		if err := r.client.Create(ctx, dv); err != nil {
			return syncState, fmt.Errorf("unable to create DV %q: %w", dv.Name, err)
		}

		syncState.DV, err = FetchObject(ctx, req.NamespacedName, r.client, &cdiv1.DataVolume{})
		if err != nil {
			return syncState, err
		}
		if dv == nil {
			return syncState, fmt.Errorf("failed to get just created dv %q", dv.Name)
		}
		log.Info(fmt.Sprintf("Creating new DV %q => %v", dv.Name, dv))
	}

	return syncState, nil
}

func (r *VMDReconciler) doSyncApplyMutated(ctx context.Context, log logr.Logger, syncState VMDReconcilerSyncState) error {
	if syncState.VMD == nil || syncState.VMDMutated == nil {
		return nil
	}
	if !reflect.DeepEqual(syncState.VMD.Status, syncState.VMD.Status) {
		return fmt.Errorf("status update is not allowed in sync phase")
	}
	if !reflect.DeepEqual(syncState.VMD.ObjectMeta, syncState.VMDMutated.ObjectMeta) {
		if err := r.updateObj(ctx, syncState.VMDMutated); err != nil {
			r.log.Error(err, "Unable to sync update VMD meta", "name", syncState.VMDMutated.Name)
			return err
		}
	}
	return nil
}

func (r *VMDReconciler) updateObj(ctx context.Context, vmd *virtv2.VirtualMachineDisk) error {
	return r.client.Update(ctx, vmd)
}

func (r *VMDReconciler) updateStatus(ctx context.Context, log logr.Logger, req reconcile.Request, syncPhaseResult *VMDReconcilerSyncPhaseResult) (VMDReconcilerUpdateStatusState, error) {
	updateStatusState := VMDReconcilerUpdateStatusState{}
	if cs, err := r.newState(ctx, log, req); err != nil {
		return VMDReconcilerUpdateStatusState{}, err
	} else if cs == nil {
		log.Info(fmt.Sprintf("Reconcile observe absent VMD: %s, it may be deleted", req.String()))
		return VMDReconcilerUpdateStatusState{}, nil
	} else {
		updateStatusState.VMDReconcilerState = *cs
	}

	if syncPhaseResult != nil {
		updateStatusState.VMDMutated.Status.Phase = syncPhaseResult.Phase
		if err := r.applyUpdateStatus(ctx, log, updateStatusState); err != nil {
			return VMDReconcilerUpdateStatusState{}, err
		}
		return updateStatusState, nil
	}

	if updateStatusState.DV == nil {
		log.Info(fmt.Sprintf("Lost DataVolume, will skip update status"))
		return VMDReconcilerUpdateStatusState{}, nil
	}

	if err := r.doUpdateStatus(ctx, log, req, &updateStatusState); err != nil {
		return VMDReconcilerUpdateStatusState{}, err
	}
	if err := r.applyUpdateStatus(ctx, log, updateStatusState); err != nil {
		return VMDReconcilerUpdateStatusState{}, fmt.Errorf("unable to apply status update of %q: %w", req.NamespacedName, err)
	}
	return updateStatusState, nil
}

func MapDataVolumePhaseToVMDPhase(phase cdiv1.DataVolumePhase) virtv2.DiskPhase {
	switch phase {
	case cdiv1.PhaseUnset, cdiv1.Unknown, cdiv1.Pending:
		return virtv2.DiskPending
	case cdiv1.WaitForFirstConsumer, cdiv1.PVCBound,
		cdiv1.ImportScheduled, cdiv1.CloneScheduled, cdiv1.UploadScheduled,
		cdiv1.ImportInProgress, cdiv1.CloneInProgress,
		cdiv1.SnapshotForSmartCloneInProgress, cdiv1.SmartClonePVCInProgress,
		cdiv1.CSICloneInProgress,
		cdiv1.CloneFromSnapshotSourceInProgress,
		cdiv1.Paused:
		return virtv2.DiskProvisioning
	case cdiv1.Succeeded:
		return virtv2.DiskReady
	case cdiv1.Failed:
		return virtv2.DiskFailed
	default:
		panic(fmt.Sprintf("unexpected DataVolume phase %q, please report a bug", phase))
	}
}

func (r *VMDReconciler) doUpdateStatus(ctx context.Context, log logr.Logger, req reconcile.Request, updateStatusState *VMDReconcilerUpdateStatusState) error {
	switch updateStatusState.VMD.Status.Phase {
	case "", virtv2.DiskPending:
		updateStatusState.VMDMutated.Status.Phase = MapDataVolumePhaseToVMDPhase(updateStatusState.DV.Status.Phase)
		updateStatusState.VMDMutated.Status.Progress = "N/A"
	case virtv2.DiskWaitForUserUpload:
	// TODO
	case virtv2.DiskProvisioning:
		updateStatusState.VMDMutated.Status.Progress = virtv2.DiskProgress(updateStatusState.DV.Status.Progress)
		updateStatusState.VMDMutated.Status.Phase = MapDataVolumePhaseToVMDPhase(updateStatusState.DV.Status.Phase)
	case virtv2.DiskReady:
		// TODO
	case virtv2.DiskFailed:
		// TODO
	case virtv2.DiskNotReady:
		// TODO
	case virtv2.DiskPVCLost:
		// TODO
	}
	return nil
}

func (r *VMDReconciler) applyUpdateStatus(ctx context.Context, log logr.Logger, updateStatusState VMDReconcilerUpdateStatusState) error {
	// TODO: update only if status changed, see DataVolume for exmpl
	if err := r.client.Status().Update(ctx, updateStatusState.VMDMutated); err != nil {
		log.Error(err, "unable to update VMD status", "name", updateStatusState.VMDMutated.Name)
		return err
	}
	return nil
}

func NewDVFromVirtualMachineDisk(namespace, name string, vmd *virtv2.VirtualMachineDisk) *cdiv1.DataVolume {
	labels := map[string]string{}
	annotations := map[string]string{
		"cdi.kubevirt.io/storage.deleteAfterCompletion":    "false",
		"cdi.kubevirt.io/storage.bind.immediate.requested": "true",
	}

	// FIXME: resource.Quantity should be defined directly in the spec struct (see PVC impl. for details)
	pvcSize, err := resource.ParseQuantity(vmd.Spec.PersistentVolumeClaim.Size)
	if err != nil {
		panic(err.Error())
	}

	res := &cdiv1.DataVolume{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   namespace,
			Name:        name,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: cdiv1.DataVolumeSpec{
			Source: &cdiv1.DataVolumeSource{},
			PVC: &corev1.PersistentVolumeClaimSpec{
				StorageClassName: &vmd.Spec.PersistentVolumeClaim.StorageClassName,
				AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}, // TODO: ensure this mode is appropriate
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: pvcSize,
					},
				},
			},
		},
	}

	if vmd.Spec.DataSource.HTTP != nil {
		res.Spec.Source.HTTP = &cdiv1.DataVolumeSourceHTTP{
			URL: vmd.Spec.DataSource.HTTP.URL,
		}
	}

	res.OwnerReferences = []metav1.OwnerReference{
		*metav1.NewControllerRef(vmd, schema.GroupVersionKind{
			Group:   virtv2.SchemeGroupVersion.Group,
			Version: virtv2.SchemeGroupVersion.Version,
			Kind:    "VirtualMachineDisk",
		}),
	}

	return res
}
