package controller

import (
	"context"
	"fmt"
	virtv2 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type VMDReconcilerCore struct{}

func (r *VMDReconcilerCore) SetupController(ctx context.Context, mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(&source.Kind{Type: &virtv2.VirtualMachineDisk{}}, &handler.EnqueueRequestForObject{},
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool { return true },
		},
	); err != nil {
		return err
	}
	if err := ctr.Watch(&source.Kind{Type: &cdiv1.DataVolume{}}, &handler.EnqueueRequestForOwner{
		OwnerType:    &virtv2.VirtualMachineDisk{},
		IsController: true,
	}); err != nil {
		return err
	}

	return nil
}

func (r *VMDReconcilerCore) Sync(ctx context.Context, req reconcile.Request, state *VMDReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	if state.DV == nil {
		var err error

		// TODO: How to set custom PVC name using DataVolume spec?
		// DataVolume named after VirtualMachineDisk (?)
		dv := NewDVFromVirtualMachineDisk(req.Namespace, req.Name, state.VMD)

		if err := opts.Client.Create(ctx, dv); err != nil {
			return fmt.Errorf("unable to create DV %q: %w", dv.Name, err)
		}

		state.DV, err = FetchObject(ctx, req.NamespacedName, opts.Client, &cdiv1.DataVolume{})
		if err != nil {
			return err
		}
		if dv == nil {
			return fmt.Errorf("failed to get just created dv %q", dv.Name)
		}
		opts.Log.Info(fmt.Sprintf("Creating new DV %q => %v", dv.Name, dv))
	}

	return nil
}

func (r *VMDReconcilerCore) UpdateStatus(ctx context.Context, req reconcile.Request, state *VMDReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	if state.DV == nil {
		opts.Log.Info(fmt.Sprintf("Lost DataVolume, will skip update status"))
		return nil
	}

	switch state.VMD.Status.Phase {
	case "", virtv2.DiskPending:
		state.VMDMutated.Status.Progress = "N/A"
		state.VMDMutated.Status.Phase = MapDataVolumePhaseToVMDPhase(state.DV.Status.Phase)
	case virtv2.DiskWaitForUserUpload:
	// TODO
	case virtv2.DiskProvisioning:
		state.VMDMutated.Status.Progress = virtv2.DiskProgress(state.DV.Status.Progress)
		state.VMDMutated.Status.Phase = MapDataVolumePhaseToVMDPhase(state.DV.Status.Phase)
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

func (r *VMDReconcilerCore) NewReconcilerState(opts two_phase_reconciler.ReconcilerOptions) *VMDReconcilerState {
	return NewVMDReconcilerState(opts.Client)
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
