package controller

import (
	"context"
	"fmt"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	"github.com/deckhouse/virtualization-controller/pkg/util"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

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

type VMDReconciler struct{}

func (r *VMDReconciler) SetupController(ctx context.Context, mgr manager.Manager, ctr controller.Controller) error {
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

func (r *VMDReconciler) Sync(ctx context.Context, req reconcile.Request, state *VMDReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	if !state.VMD.Current().ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is being deleted
		if controllerutil.ContainsFinalizer(state.VMD.Current(), virtv2.FinalizerVMDCleanup) {
			// Our finalizer is present, so lets cleanup DV, PVC & PV dependencies
			if state.DV != nil {
				if controllerutil.RemoveFinalizer(state.DV, virtv2.FinalizerDVProtection) {
					if err := opts.Client.Update(ctx, state.DV); err != nil {
						return fmt.Errorf("unable to remove DV %q finalizer %q: %w", state.DV.Name, virtv2.FinalizerDVProtection, err)
					}
				}
			}
			if state.PVC != nil {
				if controllerutil.RemoveFinalizer(state.PVC, virtv2.FinalizerPVCProtection) {
					if err := opts.Client.Update(ctx, state.PVC); err != nil {
						return fmt.Errorf("unable to remove PVC %q finalizer %q: %w", state.PVC.Name, virtv2.FinalizerPVCProtection, err)
					}
				}
			}
			if state.PV != nil {
				if controllerutil.RemoveFinalizer(state.PV, virtv2.FinalizerPVProtection) {
					if err := opts.Client.Update(ctx, state.PV); err != nil {
						return fmt.Errorf("unable to remove PV %q finalizer %q: %w", state.PV.Name, virtv2.FinalizerPVProtection, err)
					}
				}
			}
			controllerutil.RemoveFinalizer(state.VMD.Changed(), virtv2.FinalizerVMDCleanup)
		}

		// Stop reconciliation as the item is being deleted
		return nil
	}

	controllerutil.AddFinalizer(state.VMD.Changed(), virtv2.FinalizerVMDCleanup)

	if dvName, hasKey := state.VMD.Current().Annotations[AnnVMDDataVolume]; !hasKey {
		if state.VMD.Changed().Annotations == nil {
			state.VMD.Changed().Annotations = make(map[string]string)
		}
		state.VMD.Changed().Annotations[AnnVMDDataVolume] = fmt.Sprintf("virtual-machine-disk-%s", uuid.NewUUID())
		opts.Log.Info("Generated DV name", "name", state.VMD.Changed().Annotations[AnnVMDDataVolume])
	} else {
		name := types.NamespacedName{Name: dvName, Namespace: req.Namespace}

		dv, err := helper.FetchObject(ctx, name, opts.Client, &cdiv1.DataVolume{})
		if err != nil {
			return fmt.Errorf("unable to get DV %q: %w", name, err)
		}

		if dv == nil {
			dv = NewDVFromVirtualMachineDisk(name, state.VMD.Current())
			if err := opts.Client.Create(ctx, dv); err != nil {
				return fmt.Errorf("unable to create DV %q: %w", dv.Name, err)
			}
			opts.Log.Info("Created new DV", "name", dv.Name, "dv", dv)
		}

		state.DV = dv
	}

	// Add DV, PVC & PV finalizers
	if state.DV != nil {
		if controllerutil.AddFinalizer(state.DV, virtv2.FinalizerDVProtection) {
			if err := opts.Client.Update(ctx, state.DV); err != nil {
				return fmt.Errorf("error setting finalizer on a DV %q: %w", state.DV.Name)
			}
		}
	}
	if state.PVC != nil {
		if controllerutil.AddFinalizer(state.PVC, virtv2.FinalizerPVCProtection) {
			if err := opts.Client.Update(ctx, state.PVC); err != nil {
				return fmt.Errorf("error setting finalizer on a PVC %q: %w", state.PVC.Name)
			}
		}
	}
	if state.PV != nil {
		if controllerutil.AddFinalizer(state.PV, virtv2.FinalizerPVProtection) {
			if err := opts.Client.Update(ctx, state.PV); err != nil {
				return fmt.Errorf("error setting finalizer on a PV %q: %w", state.PV.Name)
			}
		}
	}

	return nil
}

func (r *VMDReconciler) UpdateStatus(ctx context.Context, req reconcile.Request, state *VMDReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	opts.Log.V(2).Info("Update Status", "pvcname", state.VMD.Current().Annotations[AnnVMDDataVolume])

	// Change previous state to new
	switch state.VMD.Current().Status.Phase {
	case "":
		state.VMD.Changed().Status.Phase = virtv2.DiskPending
		state.SetReconcilerResult(&reconcile.Result{Requeue: true})
	case virtv2.DiskPending:
		if state.DV != nil {
			nextPhase := MapDataVolumePhaseToVMDPhase(state.DV.Status.Phase)
			if nextPhase == virtv2.DiskReady {
				// Make sure not to jump over DiskProvisioning state handler
				state.VMD.Changed().Status.Phase = virtv2.DiskProvisioning
				state.SetReconcilerResult(&reconcile.Result{Requeue: true})
			} else {
				state.VMD.Changed().Status.Phase = nextPhase
			}
		}
	case virtv2.DiskWaitForUserUpload:
		// TODO
	case virtv2.DiskProvisioning:
		if state.DV != nil {
			state.VMD.Changed().Status.Phase = MapDataVolumePhaseToVMDPhase(state.DV.Status.Phase)
		} else {
			opts.Log.Info("Lost DataVolume, will skip update status")
		}
	case virtv2.DiskReady:
		// TODO
	case virtv2.DiskFailed:
		// TODO
	case virtv2.DiskNotReady:
		// TODO
	case virtv2.DiskPVCLost:
		// TODO
	}

	// TODO: ensure phases switching in the predefined order, no "phase jumps over middle phase" allowed

	// Set fields after phase changed
	switch state.VMD.Changed().Status.Phase {
	case virtv2.DiskPending:
		if state.VMD.Current().Status.Progress == "" {
			state.VMD.Changed().Status.Progress = "N/A"
		}
	case virtv2.DiskWaitForUserUpload:
	case virtv2.DiskProvisioning:
		if state.DV != nil {
			progress := virtv2.DiskProgress(state.DV.Status.Progress)
			if progress == "" {
				progress = "N/A"
			}
			state.VMD.Changed().Status.Progress = progress
		}

		if state.VMD.Current().Status.Size == "" || state.VMD.Current().Status.Size == "0" {
			if state.PVC != nil {
				if state.PVC.Status.Phase == corev1.ClaimBound {
					state.VMD.Changed().Status.Size = util.GetPointer(state.PVC.Status.Capacity[corev1.ResourceStorage]).String()
				}
			}
		}

	case virtv2.DiskReady:
		if state.VMD.Current().Status.Progress != "100%" {
			state.VMD.Changed().Status.Progress = "100%"
		}
		if state.VMD.Current().Status.PersistentVolumeClaimName == "" {
			state.VMD.Changed().Status.PersistentVolumeClaimName = state.VMD.Current().Annotations[AnnVMDDataVolume]
		}
	case virtv2.DiskFailed:
	case virtv2.DiskNotReady:
	case virtv2.DiskPVCLost:
	default:
		panic(fmt.Sprintf("unexpected phase %q", state.VMD.Changed().Status.Phase))
	}

	return nil
}

func NewDVFromVirtualMachineDisk(name types.NamespacedName, vmd *virtv2.VirtualMachineDisk) *cdiv1.DataVolume {
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
			Namespace:   name.Namespace,
			Name:        name.Name,
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
