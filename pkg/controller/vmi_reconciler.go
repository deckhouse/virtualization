package controller

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	kvalidation "k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/utils/strings"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	virtv2 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	"github.com/deckhouse/virtualization-controller/pkg/common"
	vmiutil "github.com/deckhouse/virtualization-controller/pkg/common/vmi"
	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/controller/monitoring"
	"github.com/deckhouse/virtualization-controller/pkg/controller/uploader"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/util"
)

type VMIReconciler struct {
	importerImage string
	uploaderImage string
	verbose       string
	pullPolicy    string
	dvcrSettings  *cc.DVCRSettings
}

func (r *VMIReconciler) SetupController(_ context.Context, mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(source.Kind(mgr.GetCache(), &virtv2.VirtualMachineImage{}), &handler.EnqueueRequestForObject{},
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool { return true },
		},
	); err != nil {
		return fmt.Errorf("error setting watch on VMI: %w", err)
	}

	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &cdiv1.DataVolume{}),
		handler.EnqueueRequestForOwner(
			mgr.GetScheme(),
			mgr.GetRESTMapper(),
			&virtv2.VirtualMachineImage{},
			handler.OnlyControllerOwner(),
		),
	); err != nil {
		return fmt.Errorf("error setting watch on DV: %w", err)
	}

	return nil
}

// Sync starts an importer/uploader Pod or creates a DataVolume to import image into DVCR or into PVC.
// There are 3 modes of import:
// - Start and track importer/uploader Pod only (e.g. dataSource is HTTP and storage is ContainerRegistry).
// - Start importer/uploader Pod first and then create DataVolume (e.g. target size is unknown: dataSource is HTTP and storage is Kubernetes without specified size for PVC).
// - Create and track DataVolume only (e.g. dataSource is ClusterVirtualMachineImage and storage is Kubernetes).
func (r *VMIReconciler) Sync(ctx context.Context, _ reconcile.Request, state *VMIReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	switch {
	case state.IsDeletion():
		opts.Log.V(1).Info("Delete VMI, remove protective finalizers")
		return r.removeFinalizers(ctx, state, opts)
	case !state.IsProtected():
		// Set protective finalizer atomically.
		if controllerutil.AddFinalizer(state.VMI.Changed(), virtv2.FinalizerVMICleanup) {
			state.SetReconcilerResult(&reconcile.Result{Requeue: true})
			return nil
		}
	case state.IsReady():
		// Delete underlying importer/uploader Pod, Service and DataVolume and stop the reconcile process.
		if err := r.cleanup(ctx, state.VMI.Changed(), state.Client, state); err != nil {
			return err
		}
		return nil
	case state.ShouldTrackPod() && !state.IsPodComplete():
		// Start and track importer/uploader Pod.
		switch {
		case !state.IsPodInited():
			opts.Log.V(1).Info("Update annotations with Pod name and namespace")
			r.initPodName(state.VMI.Changed())
			// Update annotations and status and restart reconcile to create an importer Pod.
			state.SetReconcilerResult(&reconcile.Result{Requeue: true})
			return nil
		case state.CanStartPod():
			// Create Pod using name and namespace from annotation.
			opts.Log.V(1).Info("Start new Pod for VMI")
			// Create importer/uploader pod, make sure the VMI owns it.
			if err := r.startPod(ctx, state.VMI.Current(), opts); err != nil {
				return err
			}
			// Requeue to wait until Pod become Running.
			state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})
			return nil
		case state.Pod != nil:
			// Import is in progress, force a re-reconcile in 2 seconds to update status.
			opts.Log.V(2).Info("Requeue: wait until Pod is completed", "vmi.name", state.VMI.Current().Name)
			if err := r.ensurePodFinalizers(ctx, state, opts); err != nil {
				return err
			}
			state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})
			return nil
		}
	case !state.ShouldTrackDataVolume() && state.ShouldTrackPod() && state.IsPodComplete():
		// Proceed to UpdateStatus and requeue to handle Ready state.
		state.SetReconcilerResult(&reconcile.Result{RequeueAfter: time.Second})
		return nil
	case state.ShouldTrackDataVolume() && (!state.ShouldTrackPod() || state.IsPodComplete()):
		// Start and track DataVolume.
		switch {
		case !state.HasDataVolumeAnno():
			opts.Log.V(1).Info("Update annotations with new DataVolume name")
			r.initDataVolumeName(state.VMI.Changed())
			// Update annotations and status and restart reconcile to create an importer/uploader Pod.
			state.SetReconcilerResult(&reconcile.Result{Requeue: true})
			return nil
		case state.CanCreateDataVolume():
			opts.Log.V(1).Info("Create DataVolume for VMI")

			if err := r.createDataVolume(ctx, state.VMI.Current(), state, opts); err != nil {
				return err
			}
			// Requeue to wait until Pod become Running.
			state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})
			return nil
		case state.DV != nil:
			// Import is in progress, force a re-reconcile in 2 seconds to update status.
			opts.Log.V(2).Info("Requeue: wait until DataVolume is completed", "vmi.name", state.VMI.Current().Name)
			if err := r.ensureDVFinalizers(ctx, state, opts); err != nil {
				return err
			}
			state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})
			return nil
		}
	}

	// Report unexpected state.
	details := fmt.Sprintf("vmi.Status.Phase='%s'", state.VMI.Current().Status.Phase)
	if state.Pod != nil {
		details += fmt.Sprintf(" pod.Name='%s' pod.Status.Phase='%s'", state.Pod.Name, state.Pod.Status.Phase)
	}
	if state.DV != nil {
		details += fmt.Sprintf(" dv.Name='%s' dv.Status.Phase='%s'", state.DV.Name, state.DV.Status.Phase)
	}
	if state.PVC != nil {
		details += fmt.Sprintf(" pvc.Name='%s' pvc.Status.Phase='%s'", state.PVC.Name, state.PVC.Status.Phase)
	}
	opts.Recorder.Event(state.VMI.Current(), corev1.EventTypeWarning, virtv2.ReasonErrUnknownState, fmt.Sprintf("VMI has unexpected state, recreate it to start import again. %s", details))

	return nil
}

func (r *VMIReconciler) UpdateStatus(_ context.Context, _ reconcile.Request, state *VMIReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	opts.Log.V(2).Info("Update VMI status", "vmi.name", state.VMI.Current().GetName())

	// Do nothing if object is being deleted as any update will lead to en error.
	if state.IsDeletion() {
		return nil
	}

	// Record event if importer/uploader Pod has error.
	// TODO set Failed status if Pod restarts are greater than some threshold?
	if state.Pod != nil && len(state.Pod.Status.ContainerStatuses) > 0 {
		if state.Pod.Status.ContainerStatuses[0].LastTerminationState.Terminated != nil &&
			state.Pod.Status.ContainerStatuses[0].LastTerminationState.Terminated.ExitCode > 0 {
			opts.Recorder.Event(state.VMI.Current(), corev1.EventTypeWarning, virtv2.ReasonErrImportFailed, fmt.Sprintf("pod phase '%s', message '%s'", state.Pod.Status.Phase, state.Pod.Status.ContainerStatuses[0].LastTerminationState.Terminated.Message))
		}
	}

	vmiStatus := state.VMI.Current().Status.DeepCopy()

	switch {
	case vmiStatus.Phase == "":
		vmiStatus.Phase = virtv2.ImagePending
		state.SetReconcilerResult(&reconcile.Result{Requeue: true})
	case state.IsReady():
		// No need to update status.
		break
	case state.ShouldTrackPod() && state.IsPodInProgress():
		opts.Log.V(2).Info("Fetch progress from Pod", "vmi.name", state.VMI.Current().GetName())

		vmiStatus.Phase = virtv2.ImageProvisioning

		if state.VMI.Current().Spec.DataSource.Type == virtv2.DataSourceTypeUpload &&
			vmiStatus.UploadCommand == "" &&
			state.Service != nil &&
			len(state.Service.Spec.Ports) > 0 {
			vmiStatus.UploadCommand = fmt.Sprintf(
				"curl -X POST --data-binary @example.iso http://%s:%d/v1beta1/upload",
				state.Service.Spec.ClusterIP,
				state.Service.Spec.Ports[0].Port,
			)
		}

		progress, err := monitoring.GetImportProgressFromPod(string(state.VMI.Current().GetUID()), state.Pod)
		if err != nil {
			opts.Recorder.Event(state.VMI.Current(), corev1.EventTypeWarning, virtv2.ReasonErrGetProgressFailed, "Error fetching progress metrics from Pod "+err.Error())
			return err
		}
		if progress != nil {
			opts.Log.V(2).Info("Got progress", "vmi.name", state.VMI.Current().Name, "progress", progress.Progress(), "speed", progress.AvgSpeed(), "progress.raw", progress.ProgressRaw(), "speed.raw", progress.AvgSpeedRaw())
			// map 0-100% to 0-50%.
			progressPct := progress.Progress()
			if state.ShouldTrackDataVolume() {
				progressPct = common.ScalePercentage(progressPct, 0, 50.0)
			}
			vmiStatus.Progress = progressPct
			vmiStatus.DownloadSpeed.Avg = progress.AvgSpeed()
			vmiStatus.DownloadSpeed.AvgBytes = strconv.FormatUint(progress.AvgSpeedRaw(), 10)
			vmiStatus.DownloadSpeed.Current = progress.CurrentSpeed()
			vmiStatus.DownloadSpeed.CurrentBytes = strconv.FormatUint(progress.CurrentSpeedRaw(), 10)
		}
	case !state.ShouldTrackDataVolume() && state.ShouldTrackPod() && state.IsPodComplete():
		vmiStatus.Phase = virtv2.ImageReady

		opts.Log.V(1).Info("Import completed successfully")

		opts.Recorder.Event(state.VMI.Current(), corev1.EventTypeNormal, virtv2.ReasonImportSucceeded, "Import Successful")

		vmiStatus.Progress = ""

		vmiStatus.DownloadSpeed = virtv2.ImageStatusSpeed{}

		finalReport, err := monitoring.GetFinalReportFromPod(state.Pod)
		if err != nil {
			opts.Log.Error(err, "parsing final report", "vmi.name", state.VMI.Current().Name)
		}
		if finalReport != nil {
			vmiStatus.Size.Stored = finalReport.StoredSize()
			vmiStatus.Size.StoredBytes = strconv.FormatUint(finalReport.StoredSizeBytes, 10)
			vmiStatus.Size.Unpacked = finalReport.UnpackedSize()
			vmiStatus.Size.UnpackedBytes = strconv.FormatUint(finalReport.UnpackedSizeBytes, 10)
		}
		// Set target image name the same way as for the importer/uploader Pod.
		vmiStatus.Target.RegistryURL = cc.PrepareDVCREndpointFromVMI(state.VMI.Current(), r.dvcrSettings)
	case state.ShouldTrackDataVolume() && state.IsDataVolumeInProgress():
		// Set phase from DataVolume resource.
		vmiStatus.Phase = MapDataVolumePhaseToVMIPhase(state.DV.Status.Phase)

		// Download speed is not available from DataVolume.
		vmiStatus.DownloadSpeed = virtv2.ImageStatusSpeed{}

		// Copy progress from DataVolume.
		// map 0-100% to 50%-100%.
		dvProgress := string(state.DV.Status.Progress)

		opts.Log.V(2).Info("Got DataVolume progress", "progress", dvProgress)

		if dvProgress != "N/A" && dvProgress != "" {
			vmiStatus.Progress = common.ScalePercentage(dvProgress, 50.0, 100.0)
		}

		// Copy capacity from PVC.
		if state.PVC != nil && state.PVC.Status.Phase == corev1.ClaimBound {
			vmiStatus.Capacity = util.GetPointer(state.PVC.Status.Capacity[corev1.ResourceStorage]).String()
		}
	case state.ShouldTrackDataVolume() && state.IsDataVolumeComplete():
		opts.Recorder.Event(state.VMI.Current(), corev1.EventTypeNormal, virtv2.ReasonImportSucceededToPVC, "Import Successful")
		opts.Log.V(1).Info("Import completed successfully")
		vmiStatus.Phase = virtv2.ImageReady

		// Cleanup.
		vmiStatus.Progress = ""
		vmiStatus.DownloadSpeed = virtv2.ImageStatusSpeed{}
		// PVC name is the same as the DataVolume name.
		vmiStatus.Target.PersistentVolumeClaimName = state.VMI.Current().Annotations[cc.AnnVMIDataVolume]
	}

	state.VMI.Changed().Status = *vmiStatus

	return nil
}

func MapDataVolumePhaseToVMIPhase(phase cdiv1.DataVolumePhase) virtv2.ImagePhase {
	switch phase {
	case cdiv1.PhaseUnset, cdiv1.Unknown, cdiv1.Pending:
		return virtv2.ImagePending
	case cdiv1.WaitForFirstConsumer, cdiv1.PVCBound,
		cdiv1.ImportScheduled, cdiv1.CloneScheduled, cdiv1.UploadScheduled,
		cdiv1.ImportInProgress, cdiv1.CloneInProgress,
		cdiv1.SnapshotForSmartCloneInProgress, cdiv1.SmartClonePVCInProgress,
		cdiv1.CSICloneInProgress,
		cdiv1.CloneFromSnapshotSourceInProgress,
		cdiv1.Paused:
		return virtv2.ImageProvisioning
	case cdiv1.Succeeded:
		return virtv2.ImageReady
	case cdiv1.Failed:
		return virtv2.ImageFailed
	default:
		panic(fmt.Sprintf("unexpected DataVolume phase %q, please report a bug", phase))
	}
}

// ensurePodFinalizers adds protective finalizers on importer/uploader Pod and Service dependencies.
func (r *VMIReconciler) ensurePodFinalizers(ctx context.Context, state *VMIReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	if state.Pod != nil {
		if controllerutil.AddFinalizer(state.Pod, virtv2.FinalizerPodProtection) {
			if err := opts.Client.Update(ctx, state.Pod); err != nil {
				return fmt.Errorf("error setting finalizer on a Pod %q: %w", state.Pod.Name, err)
			}
		}
	}
	if state.Service != nil {
		if controllerutil.AddFinalizer(state.Service, virtv2.FinalizerServiceProtection) {
			if err := opts.Client.Update(ctx, state.Service); err != nil {
				return fmt.Errorf("error setting finalizer on a Service %q: %w", state.Service.Name, err)
			}
		}
	}

	return nil
}

// ensureDVFinalizers adds protective finalizers on DataVolume, PersistentVolumeClaim and PersistentVolume dependencies.
func (r *VMIReconciler) ensureDVFinalizers(ctx context.Context, state *VMIReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	if state.DV != nil {
		// Ensure DV finalizer is set in case DV was created manually (take ownership of already existing object)
		if controllerutil.AddFinalizer(state.DV, virtv2.FinalizerDVProtection) {
			if err := opts.Client.Update(ctx, state.DV); err != nil {
				return fmt.Errorf("error setting finalizer on a DV %q: %w", state.DV.Name, err)
			}
		}
	}
	if state.PVC != nil {
		if controllerutil.AddFinalizer(state.PVC, virtv2.FinalizerPVCProtection) {
			if err := opts.Client.Update(ctx, state.PVC); err != nil {
				return fmt.Errorf("error setting finalizer on a PVC %q: %w", state.PVC.Name, err)
			}
		}
	}
	if state.PV != nil {
		if controllerutil.AddFinalizer(state.PV, virtv2.FinalizerPVProtection) {
			if err := opts.Client.Update(ctx, state.PV); err != nil {
				return fmt.Errorf("error setting finalizer on a PV %q: %w", state.PV.Name, err)
			}
		}
	}

	return nil
}

// removeFinalizers removes protective finalizers on Pod, Service, DataVolume, PersistentVolumeClaim and PersistentVolume dependencies.
func (r *VMIReconciler) removeFinalizers(ctx context.Context, state *VMIReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	if state.Pod != nil {
		if controllerutil.RemoveFinalizer(state.Pod, virtv2.FinalizerPodProtection) {
			if err := opts.Client.Update(ctx, state.Pod); err != nil {
				return fmt.Errorf("unable to remove Pod %q finalizer %q: %w", state.Pod.Name, virtv2.FinalizerPodProtection, err)
			}
		}
	}
	if state.Service != nil {
		if controllerutil.RemoveFinalizer(state.Service, virtv2.FinalizerServiceProtection) {
			if err := opts.Client.Update(ctx, state.Service); err != nil {
				return fmt.Errorf("unable to remove Service %q finalizer %q: %w", state.Service.Name, virtv2.FinalizerServiceProtection, err)
			}
		}
	}
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
	controllerutil.RemoveFinalizer(state.VMI.Changed(), virtv2.FinalizerVMICleanup)

	return nil
}

// initPodName creates new name and update it in the annotation.
func (r *VMIReconciler) initPodName(vmi *virtv2.VirtualMachineImage) {
	anno := vmi.GetAnnotations()
	if anno == nil {
		anno = make(map[string]string)
	}

	switch vmi.Spec.DataSource.Type {
	case virtv2.DataSourceTypeUpload:
		anno[cc.AnnUploadPodName] = fmt.Sprintf("%s-%s", common.UploaderPodNamePrefix, vmi.GetName())
		anno[cc.AnnUploadServiceName] = fmt.Sprintf("%s-%s", common.UploaderServiceNamePrefix, vmi.GetName())
	default:
		anno[cc.AnnImportPodName] = fmt.Sprintf("%s-%s", common.ImporterPodNamePrefix, vmi.GetName())
		// Generate name for secret with certs from caBundle.
		if vmiutil.HasCABundle(vmi) {
			anno[cc.AnnCABundleConfigMap] = fmt.Sprintf("%s-ca", vmi.GetName())
		}
	}

	vmi.SetAnnotations(anno)
}

func (r *VMIReconciler) startPod(ctx context.Context, vmi *virtv2.VirtualMachineImage, opts two_phase_reconciler.ReconcilerOptions) error {
	switch vmi.Spec.DataSource.Type {
	case virtv2.DataSourceTypeUpload:
		if err := r.startUploaderPod(ctx, vmi, opts); err != nil {
			return err
		}

		if err := r.startUploaderService(ctx, vmi, opts); err != nil {
			return err
		}
	default:
		if err := r.startImporterPod(ctx, vmi, opts); err != nil {
			return err
		}
	}

	return nil
}

// initDataVolumeName creates new DataVolume name and update it in the annotation.
func (r *VMIReconciler) initDataVolumeName(vmi *virtv2.VirtualMachineImage) {
	// Prevent DataVolume name regeneration.
	if _, hasKey := vmi.Annotations[cc.AnnVMIDataVolume]; hasKey {
		return
	}

	anno := vmi.GetAnnotations()
	if anno == nil {
		anno = make(map[string]string)
	}

	// Generate DataVolume name.
	// FIXME: move shortening to separate method. (See https://github.com/deckhouse/3p-containerized-data-importer/blob/ab8b9c025e40b43272a433c600c107cb993ebf90/pkg/util/naming/namer.go).
	anno[cc.AnnVMIDataVolume] = strings.ShortenString(fmt.Sprintf("vmi-%s-%s", vmi.GetName(), uuid.NewUUID()), kvalidation.DNS1123SubdomainMaxLength)

	vmi.Annotations = anno
}

func (r *VMIReconciler) createDataVolume(ctx context.Context, vmi *virtv2.VirtualMachineImage, state *VMIReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	dvName := types.NamespacedName{Name: vmi.GetAnnotations()[cc.AnnVMIDataVolume], Namespace: vmi.GetNamespace()}
	dvBuilder := kvbuilder.NewDV(dvName)

	finalReport, err := monitoring.GetFinalReportFromPod(state.Pod)
	if err != nil {
		return fmt.Errorf("cannot create DV without final report from the Pod: %w", err)
	}

	if finalReport.UnpackedSizeBytes == 0 {
		return errors.New("no unpacked size in final report")
	}

	pvcSize := *resource.NewQuantity(int64(finalReport.UnpackedSizeBytes), resource.BinarySI)

	err = kvbuilder.ApplyVirtualMachineImageSpec(dvBuilder, vmi, pvcSize, r.dvcrSettings)
	if err != nil {
		return fmt.Errorf("apply VMI spec to DataVolume: %w", err)
	}
	dv := dvBuilder.GetResource()

	if err = opts.Client.Create(ctx, dv); err != nil {
		opts.Log.V(2).Info("Error create new DV spec", "dv.spec", dv.Spec)
		return fmt.Errorf("create DataVolume/%s for VMI/%s: %w", dv.GetName(), vmi.GetName(), err)
	}
	opts.Log.Info("Created new DV", "dv.name", dv.GetName())
	opts.Log.V(2).Info("Created new DV spec", "dv.spec", dv.Spec)
	return nil
}

func (r *VMIReconciler) cleanup(ctx context.Context, vmi *virtv2.VirtualMachineImage, client client.Client, state *VMIReconcilerState) error {
	if state.DV != nil {
		if err := client.Delete(ctx, state.DV); err != nil {
			return fmt.Errorf("cleanup DataVolume: %w", err)
		}
	}

	if state.Pod != nil && cc.ShouldDeletePod(state.VMI.Current()) {
		switch vmi.Spec.DataSource.Type {
		case virtv2.DataSourceTypeUpload:
			if err := uploader.CleanupService(ctx, client, state.Service); err != nil {
				return err
			}

			if err := uploader.CleanupPod(ctx, client, state.Pod); err != nil {
				return err
			}
		default:
			if err := importer.CleanupPod(ctx, client, state.Pod); err != nil {
				return err
			}
		}
	}

	return nil
}
