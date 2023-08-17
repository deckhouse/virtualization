package controller

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/dustin/go-humanize"
	corev1 "k8s.io/api/core/v1"
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
	vmdutil "github.com/deckhouse/virtualization-controller/pkg/common/vmd"
	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/controller/monitoring"
	"github.com/deckhouse/virtualization-controller/pkg/controller/uploader"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmattachee"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/util"
)

type VMDReconciler struct {
	*vmattachee.AttacheeReconciler[*virtv2.VirtualMachineDisk, virtv2.VirtualMachineDiskStatus]

	importerImage string
	uploaderImage string
	verbose       string
	pullPolicy    string
	dvcrSettings  *cc.DVCRSettings
}

func NewVMDReconciler(importerImage, uploaderImage, verbose, pullPolicy string, dvcrSettings *cc.DVCRSettings) *VMDReconciler {
	return &VMDReconciler{
		importerImage: importerImage,
		uploaderImage: uploaderImage,
		verbose:       verbose,
		pullPolicy:    pullPolicy,
		dvcrSettings:  dvcrSettings,
		AttacheeReconciler: vmattachee.NewAttacheeReconciler[
			*virtv2.VirtualMachineDisk,
			virtv2.VirtualMachineDiskStatus,
		]("vmd", true),
	}
}

func (r *VMDReconciler) SetupController(ctx context.Context, mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(source.Kind(mgr.GetCache(), &virtv2.VirtualMachineDisk{}), &handler.EnqueueRequestForObject{},
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool { return true },
		},
	); err != nil {
		return fmt.Errorf("error setting watch on VMD: %w", err)
	}

	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &cdiv1.DataVolume{}),
		handler.EnqueueRequestForOwner(
			mgr.GetScheme(),
			mgr.GetRESTMapper(),
			&virtv2.VirtualMachineDisk{},
			handler.OnlyControllerOwner(),
		),
	); err != nil {
		return fmt.Errorf("error setting watch on DV: %w", err)
	}

	return r.AttacheeReconciler.SetupController(ctx, mgr, ctr)
}

// Sync starts an importer Pod and creates a DataVolume to import image into PVC.
func (r *VMDReconciler) Sync(ctx context.Context, _ reconcile.Request, state *VMDReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	log := opts.Log.WithValues("vmd.name", state.VMD.Current().GetName())

	log.V(2).Info("Sync VMD")

	if r.AttacheeReconciler.Sync(ctx, state.AttacheeState, opts) {
		return nil
	}

	switch {
	case state.IsDeletion():
		log.V(1).Info("Delete VMD, remove protective finalizers")
		return r.removeFinalizers(ctx, state, opts)
	case !state.IsProtected():
		// Set protective finalizer atomically.
		if controllerutil.AddFinalizer(state.VMD.Changed(), virtv2.FinalizerVMDCleanup) {
			state.SetReconcilerResult(&reconcile.Result{Requeue: true})
			return nil
		}
	case state.IsReady():
		// Delete underlying importer/uploader Pod, Service and DataVolume and stop the reconcile process.
		if err := r.cleanup(ctx, state.VMD.Changed(), state.Client, state); err != nil {
			return err
		}
		return nil
	case state.ShouldTrackPod() && !state.IsPodComplete():
		// Start and track importer/uploader Pod.
		switch {
		case !state.IsPodInited():
			log.V(1).Info("Update annotations with importer/uploader Pod name and namespace")
			if err := r.initPodName(state.VMD.Changed()); err != nil {
				return err
			}
			// Update annotations and status and restart reconcile to create an importer/uploader Pod.
			state.SetReconcilerResult(&reconcile.Result{Requeue: true})
			return nil
		case state.CanStartPod():
			// Create Pod using name and namespace from annotation.
			log.V(1).Info("Start new Pod for VMD")
			// Create importer/uploader pod, make sure the VMD owns it.
			if err := r.startPod(ctx, state.VMD.Current(), opts); err != nil {
				return err
			}
			// Requeue to wait until Pod become Running.
			state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})
			return nil
		case state.Pod != nil:
			// Import is in progress, force a re-reconcile in 2 seconds to update status.
			log.V(2).Info("Requeue: wait until Pod is completed", "vmd.name", state.VMD.Current().Name)
			if err := r.ensurePodFinalizers(ctx, state, opts); err != nil {
				return err
			}
			state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})
			return nil
		}
	case state.ShouldTrackDataVolume() && (!state.ShouldTrackPod() || state.IsPodComplete()):
		// Start and track DataVolume.
		switch {
		case !state.HasDataVolumeAnno():
			log.V(1).Info("Update annotations with new DataVolume name")
			r.initDataVolumeName(state.VMD.Changed())
			// Update annotations and status and restart reconcile to create a DV.
			state.SetReconcilerResult(&reconcile.Result{Requeue: true})
			return nil
		case state.CanCreateDataVolume():
			log.V(1).Info("Create DataVolume for VMD")

			if err := r.createDataVolume(ctx, state.VMD.Current(), state, opts); err != nil {
				return err
			}
			// Requeue to wait until Pod become Running.
			state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})
			return nil
		case state.DV != nil:
			// Import is in progress, force a re-reconcile in 2 seconds to update status.
			log.V(2).Info("Requeue: wait until DataVolume is completed", "vmd.name", state.VMD.Current().Name)
			if err := r.ensureDVFinalizers(ctx, state, opts); err != nil {
				return err
			}
			state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})
			return nil
		}
	}

	// Report unexpected state.
	details := fmt.Sprintf("vmd.Status.Phase='%s'", state.VMD.Current().Status.Phase)
	if state.Pod != nil {
		details += fmt.Sprintf(" pod.Name='%s' pod.Status.Phase='%s'", state.Pod.Name, state.Pod.Status.Phase)
	}
	if state.DV != nil {
		details += fmt.Sprintf(" dv.Name='%s' dv.Status.Phase='%s'", state.DV.Name, state.DV.Status.Phase)
	}
	if state.PVC != nil {
		details += fmt.Sprintf(" pvc.Name='%s' pvc.Status.Phase='%s'", state.PVC.Name, state.PVC.Status.Phase)
	}
	opts.Recorder.Event(state.VMD.Current(), corev1.EventTypeWarning, "ErrUnknownState", fmt.Sprintf("VMD has unexpected state, recreate it to start import again. %s", details))

	return nil
}

func (r *VMDReconciler) UpdateStatus(_ context.Context, _ reconcile.Request, state *VMDReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	log := opts.Log.WithValues("vmd.name", state.VMD.Current().GetName())

	log.V(2).Info("Update VMD status")

	// Do nothing if object is being deleted as any update will lead to en error.
	if state.IsDeletion() {
		return nil
	}

	// Record event if importer/uploader Pod has error.
	// TODO set Failed status if Pod restarts are greater than some threshold?
	if state.Pod != nil && len(state.Pod.Status.ContainerStatuses) > 0 {
		if state.Pod.Status.ContainerStatuses[0].LastTerminationState.Terminated != nil &&
			state.Pod.Status.ContainerStatuses[0].LastTerminationState.Terminated.ExitCode > 0 {
			opts.Recorder.Event(state.VMD.Current(), corev1.EventTypeWarning, "ErrImportFailed", fmt.Sprintf("importer pod phase '%s', message '%s'", state.Pod.Status.Phase, state.Pod.Status.ContainerStatuses[0].LastTerminationState.Terminated.Message))
		}
	}

	vmdStatus := state.VMD.Current().Status.DeepCopy()

	switch {
	case vmdStatus.Phase == "":
		vmdStatus.Phase = virtv2.DiskPending
		state.SetReconcilerResult(&reconcile.Result{Requeue: true})
	case state.IsReady():
		// No need to update status.
		break
	case state.ShouldTrackPod() && state.IsPodInProgress():
		log.V(2).Info("Fetch progress from Pod")

		vmdStatus.Phase = virtv2.DiskProvisioning

		if state.VMD.Current().Spec.DataSource != nil &&
			state.VMD.Current().Spec.DataSource.Type == virtv2.DataSourceTypeUpload &&
			vmdStatus.UploadCommand == "" &&
			state.Service != nil &&
			len(state.Service.Spec.Ports) > 0 {
			vmdStatus.UploadCommand = fmt.Sprintf(
				"curl -X POST --data-binary @example.iso http://%s:%d/v1beta1/upload",
				state.Service.Spec.ClusterIP,
				state.Service.Spec.Ports[0].Port,
			)
		}

		progress, err := monitoring.GetImportProgressFromPod(string(state.VMD.Current().GetUID()), state.Pod)
		if err != nil {
			opts.Recorder.Event(state.VMD.Current(), corev1.EventTypeWarning, "ErrGetProgressFailed", "Error fetching progress metrics from Pod "+err.Error())
			return err
		}
		if progress != nil {
			log.V(2).Info("Got Pod progress", "progress", progress.Progress(), "speed", progress.AvgSpeed(), "progress.raw", progress.ProgressRaw(), "speed.raw", progress.AvgSpeedRaw())
			// map 0-100% to 0-50%.
			progressPct := progress.Progress()
			if state.ShouldTrackDataVolume() {
				progressPct = common.ScalePercentage(progressPct, 0, 50.0)
			}
			vmdStatus.Progress = progressPct
			vmdStatus.DownloadSpeed.Avg = progress.AvgSpeed()
			vmdStatus.DownloadSpeed.Current = progress.CurrentSpeed()
		}
	case state.ShouldTrackDataVolume() && state.IsDataVolumeInProgress():
		// Set phase from DataVolume resource.
		vmdStatus.Phase = MapDataVolumePhaseToVMDPhase(state.DV.Status.Phase)

		// Download speed is not available from DataVolume.
		vmdStatus.DownloadSpeed = virtv2.VMDDownloadSpeed{}

		// Copy progress from DataVolume.
		// map 0-100% to 50%-100%.
		dvProgress := string(state.DV.Status.Progress)

		opts.Log.V(2).Info("Got DataVolume progress", "progress", dvProgress)

		if dvProgress != "N/A" && dvProgress != "" {
			vmdStatus.Progress = common.ScalePercentage(dvProgress, 50.0, 100.0)
		}

		// Copy capacity from PVC.
		if state.PVC != nil && state.PVC.Status.Phase == corev1.ClaimBound {
			vmdStatus.Capacity = util.GetPointer(state.PVC.Status.Capacity[corev1.ResourceStorage]).String()
		}
	case state.ShouldTrackDataVolume() && state.IsDataVolumeComplete():
		log.V(1).Info("Import completed successfully")

		vmdStatus.Phase = virtv2.DiskReady

		opts.Recorder.Event(state.VMD.Current(), corev1.EventTypeNormal, "ImportSucceeded", "Successfully imported")

		// Cleanup.
		vmdStatus.Progress = ""
		vmdStatus.DownloadSpeed = virtv2.VMDDownloadSpeed{}
		// PVC name is the same as the DataVolume name.
		vmdStatus.Target.PersistentVolumeClaimName = state.VMD.Current().Annotations[cc.AnnVMDDataVolume]
	}

	state.VMD.Changed().Status = *vmdStatus

	return nil
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

// ensurePodFinalizers adds protective finalizers on importer/uploader Pod and Service dependencies.
func (r *VMDReconciler) ensurePodFinalizers(ctx context.Context, state *VMDReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
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
func (r *VMDReconciler) ensureDVFinalizers(ctx context.Context, state *VMDReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
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
func (r *VMDReconciler) removeFinalizers(ctx context.Context, state *VMDReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
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
	controllerutil.RemoveFinalizer(state.VMD.Changed(), virtv2.FinalizerVMDCleanup)

	return nil
}

// initPodName creates new name and update it in the annotation.
func (r *VMDReconciler) initPodName(vmd *virtv2.VirtualMachineDisk) error {
	if vmd.Spec.DataSource == nil {
		return fmt.Errorf("unexpected nil spec.dataSource to init pod name")
	}

	anno := vmd.GetAnnotations()
	if anno == nil {
		anno = make(map[string]string)
	}

	switch vmd.Spec.DataSource.Type {
	case virtv2.DataSourceTypeUpload:
		anno[cc.AnnUploadPodName] = fmt.Sprintf("%s-%s", common.UploaderPodNamePrefix, vmd.GetName())
		anno[cc.AnnUploadServiceName] = fmt.Sprintf("%s-%s", common.UploaderServiceNamePrefix, vmd.GetName())
	default:
		anno[cc.AnnImportPodName] = fmt.Sprintf("%s-%s", common.ImporterPodNamePrefix, vmd.GetName())
		// Generate name for secret with certs from caBundle.
		if vmdutil.HasCABundle(vmd) {
			anno[cc.AnnCABundleConfigMap] = fmt.Sprintf("%s-ca", vmd.GetName())
		}
	}

	vmd.SetAnnotations(anno)

	return nil
}

func (r *VMDReconciler) startPod(ctx context.Context, vmd *virtv2.VirtualMachineDisk, opts two_phase_reconciler.ReconcilerOptions) error {
	if vmd.Spec.DataSource == nil {
		return fmt.Errorf("unexpected nil spec.dataSource to start pod")
	}

	switch vmd.Spec.DataSource.Type {
	case virtv2.DataSourceTypeUpload:
		if err := r.startUploaderPod(ctx, vmd, opts); err != nil {
			return err
		}

		if err := r.startUploaderService(ctx, vmd, opts); err != nil {
			return err
		}
	default:
		if err := r.startImporterPod(ctx, vmd, opts); err != nil {
			return err
		}
	}

	return nil
}

// initDataVolumeName creates new DV name and update it in the annotation.
func (r *VMDReconciler) initDataVolumeName(vmd *virtv2.VirtualMachineDisk) {
	// Prevent DataVolume name regeneration.
	if _, hasKey := vmd.Annotations[cc.AnnVMDDataVolume]; hasKey {
		return
	}

	anno := vmd.GetAnnotations()
	if anno == nil {
		anno = make(map[string]string)
	}

	// Generate DataVolume name.
	// FIXME: move shortening to separate method. (See https://github.com/deckhouse/3p-containerized-data-importer/blob/ab8b9c025e40b43272a433c600c107cb993ebf90/pkg/util/naming/namer.go).
	anno[cc.AnnVMDDataVolume] = strings.ShortenString(fmt.Sprintf("vmd-%s-%s", vmd.GetName(), uuid.NewUUID()), kvalidation.DNS1123SubdomainMaxLength)

	vmd.Annotations = anno
}

func (r *VMDReconciler) createDataVolume(ctx context.Context, vmd *virtv2.VirtualMachineDisk, state *VMDReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	dvName := types.NamespacedName{Name: vmd.GetAnnotations()[cc.AnnVMDDataVolume], Namespace: vmd.GetNamespace()}
	dvBuilder := kvbuilder.NewDV(dvName)

	finalReport, err := monitoring.GetFinalReportFromPod(state.Pod)
	if err != nil {
		return err
	}

	pvcSize := vmd.Spec.PersistentVolumeClaim.Size

	// TODO process case with missing spec.pvc.size and empty final report.
	if finalReport != nil {
		if pvcSize == "" {
			// Set the resulting size from the importer/uploader pod.
			pvcSize = strconv.FormatUint(finalReport.UnpackedSizeBytes, 10)
		} else {
			// Validate specified spec.PersistentVolumeClaim.size.
			parsedSize, err := humanize.ParseBytes(vmd.Spec.PersistentVolumeClaim.Size)
			if err != nil {
				return err
			}

			if parsedSize < finalReport.StoredSizeBytes {
				opts.Recorder.Event(state.VMD.Current(), corev1.EventTypeWarning, "WrongPVCSize", "The specified spec.PersistentVolumeClaim.size cannot be smaller than the size of image in spec.dataSource")

				return errors.New("the specified spec.PersistentVolumeClaim.size cannot be smaller than the size of image in spec.dataSource")
			}
		}
	}

	err = kvbuilder.ApplyVirtualMachineDiskSpec(dvBuilder, vmd, pvcSize, r.dvcrSettings)
	if err != nil {
		return fmt.Errorf("apply VMD spec to DataVolume: %w", err)
	}

	dv := dvBuilder.GetResource()

	if err = opts.Client.Create(ctx, dv); err != nil {
		opts.Log.V(2).Info("Error create new DV spec", "dv.spec", dv.Spec)
		return fmt.Errorf("create DataVolume/%s for VMD/%s: %w", dv.GetName(), vmd.GetName(), err)
	}
	opts.Log.Info("Created new DV", "dv.name", dv.GetName())
	opts.Log.V(2).Info("Created new DV spec", "dv.spec", dv.Spec)
	return nil
}

func (r *VMDReconciler) cleanup(ctx context.Context, vmd *virtv2.VirtualMachineDisk, client client.Client, state *VMDReconcilerState) error {
	if state.DV != nil {
		if err := client.Delete(ctx, state.DV); err != nil {
			return fmt.Errorf("cleanup DataVolume: %w", err)
		}
	}

	if state.Pod != nil && cc.ShouldDeletePod(state.VMD.Current()) {
		if vmd.Spec.DataSource == nil {
			return fmt.Errorf("unexpected nil spec.dataSource to cleanup")
		}

		switch vmd.Spec.DataSource.Type {
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
