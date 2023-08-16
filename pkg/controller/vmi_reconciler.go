package controller

import (
	"context"
	"fmt"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	kvalidation "k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/utils/strings"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
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
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/util"
)

type VMIReconciler struct {
	image        string
	verbose      string
	pullPolicy   string
	dvcrSettings *cc.DVCRSettings
}

func (r *VMIReconciler) SetupController(_ context.Context, mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(source.Kind(mgr.GetCache(), &virtv2.VirtualMachineImage{}), &handler.EnqueueRequestForObject{},
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
			&virtv2.VirtualMachineImage{},
			handler.OnlyControllerOwner(),
		),
	); err != nil {
		return fmt.Errorf("error setting watch on DV: %w", err)
	}

	return nil
}

// Sync starts an importer Pod or creates a DataVolume to import image into DVCR or into PVC.
// There are 3 modes of import:
// - Start and track importer Pod only (e.g. dataSource is HTTP and storage is ContainerRegistry).
// - Start importer Pod first and then create DataVolume (e.g. target size is unknown: dataSource is HTTP and storage is Kubernetes without specified size for PVC).
// - Create and track DataVolume only (e.g. dataSource is ClusterVirtualMachineImage and storage is Kubernetes).
func (r *VMIReconciler) Sync(ctx context.Context, _ reconcile.Request, state *VMIReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	switch {
	case state.IsDeletion():
		opts.Log.V(1).Info("Delete VMI, remove protective finalizers")
		return r.removeDVFinalizers(ctx, state, opts)
	case !state.IsProtected():
		// Set protective finalizer atomically.
		if controllerutil.AddFinalizer(state.VMI.Changed(), virtv2.FinalizerVMICleanup) {
			state.SetReconcilerResult(&reconcile.Result{Requeue: true})
			return nil
		}
	case state.IsReady():
		// Delete underlying importer Pod and DataVolume and stop the reconcile process.
		if state.DV != nil {
			if err := opts.Client.Delete(ctx, state.DV); err != nil {
				return fmt.Errorf("cleanup DataVolume: %w", err)
			}
		}
		if state.Pod != nil && cc.ShouldDeleteImporterPod(state.VMI.Current()) {
			opts.Log.V(1).Info("Import done, cleanup importer Pod", "pod.Name", state.Pod.Name)
			if err := importer.CleanupPod(ctx, opts.Client, state.Pod); err != nil {
				return fmt.Errorf("cleanup importer Pod: %w", err)
			}
		}
		return nil
	case state.ShouldTrackImporterPod() && !state.IsImporterPodComplete():
		// Start and track importer Pod.
		switch {
		case !state.HasImporterPodAnno():
			opts.Log.V(1).Info("Update annotations with importer Pod name and namespace")
			// TODO(i.mikh) This algorithm is from CDI: put annotation on fresh CVMI and run Pod on next call to reconcile. Is it ok?
			r.initImporterPodName(state.VMI.Changed())
			// Update annotations and status and restart reconcile to create an importer Pod.
			state.SetReconcilerResult(&reconcile.Result{Requeue: true})
			return nil
		case state.CanStartImporterPod():
			// Create Pod using name and namespace from annotation.
			opts.Log.V(1).Info("Start new importer Pod for VMI")
			// Create importer pod, make sure the VMI owns it.
			if err := r.startImporterPod(ctx, state.VMI.Current(), opts); err != nil {
				return err
			}
			// Requeue to wait until Pod become Running.
			state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})
			return nil
		case state.Pod != nil:
			// Import is in progress, force a re-reconcile in 2 seconds to update status.
			opts.Log.V(2).Info("Requeue: wait until importer Pod is completed", "vmi.name", state.VMI.Current().Name)
			state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})
			return nil
		}
	case state.ShouldTrackImporterPod() && state.IsImporterPodComplete() && (!state.ShouldTrackDataVolume() || !state.HasTargetPVCSize()):
		// Proceed to UpdateStatus and requeue to handle Ready state.
		state.SetReconcilerResult(&reconcile.Result{RequeueAfter: time.Second})
		return nil
	case state.ShouldTrackDataVolume() && (!state.ShouldTrackImporterPod() || state.IsImporterPodComplete()):
		// Start and track DataVolume.
		switch {
		case !state.HasDataVolumeAnno():
			opts.Log.V(1).Info("Update annotations with new DataVolume name")
			r.initDataVolumeName(state.VMI.Changed())
			// Update annotations and status and restart reconcile to create an importer Pod.
			state.SetReconcilerResult(&reconcile.Result{Requeue: true})
			return nil
		case state.CanCreateDataVolume():
			opts.Log.V(1).Info("Create DataVolume for VMI")

			pvcSize := state.GetTargetPVCSize()
			if pvcSize == "" {
				return fmt.Errorf("invalid VMI/%s: neither spec.persistentVolumeClaim.size nor status.size specify the target PVC size", state.VMI.Current().GetName())
			}

			if err := r.createDataVolume(ctx, state.VMI.Current(), pvcSize, opts); err != nil {
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
	opts.Recorder.Event(state.VMI.Current(), corev1.EventTypeWarning, "ErrUnknownState", fmt.Sprintf("VMI has unexpected state, recreate it to start import again. %s", details))

	return nil
}

func (r *VMIReconciler) UpdateStatus(_ context.Context, _ reconcile.Request, state *VMIReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	opts.Log.V(2).Info("Update VMI status", "vmi.name", state.VMI.Current().GetName())

	// Do nothing if object is being deleted as any update will lead to en error.
	if state.IsDeletion() {
		return nil
	}

	// Record event if importer Pod has error.
	// TODO set Failed status if Pod restarts are greater than some threshold?
	if state.Pod != nil && len(state.Pod.Status.ContainerStatuses) > 0 {
		if state.Pod.Status.ContainerStatuses[0].LastTerminationState.Terminated != nil &&
			state.Pod.Status.ContainerStatuses[0].LastTerminationState.Terminated.ExitCode > 0 {
			opts.Recorder.Event(state.VMI.Current(), corev1.EventTypeWarning, "ErrImportFailed", fmt.Sprintf("importer pod phase '%s', message '%s'", state.Pod.Status.Phase, state.Pod.Status.ContainerStatuses[0].LastTerminationState.Terminated.Message))
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
	case state.ShouldTrackImporterPod() && state.IsImporterPodInProgress():
		// Set CVMI status to Provisioning and copy progress metrics from importer Pod.
		opts.Log.V(2).Info("Fetch progress from importer Pod", "vmi.name", state.VMI.Current().GetName())
		vmiStatus.Phase = virtv2.ImageProvisioning

		progress, err := importer.ProgressFromPod(string(state.VMI.Current().GetUID()), state.Pod)
		if err != nil {
			opts.Recorder.Event(state.VMI.Current(), corev1.EventTypeWarning, "ErrGetProgressFailed", "Error fetching progress metrics from importer Pod "+err.Error())
			return err
		}
		if progress != nil {
			opts.Log.V(2).Info("Got progress", "cvmi.name", state.VMI.Current().Name, "progress", progress.Progress(), "speed", progress.AvgSpeed(), "progress.raw", progress.ProgressRaw(), "speed.raw", progress.AvgSpeedRaw())
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

	case state.ShouldTrackImporterPod() && state.IsImporterPodComplete() && !state.HasTargetPVCSize():
		// Set VMI status to Ready and update image size from final report of the importer Pod.
		opts.Recorder.Event(state.VMI.Current(), corev1.EventTypeNormal, "ImportSucceeded", "Import Successful")
		opts.Log.V(1).Info("Import completed successfully")
		// Cleanup.
		if !state.ShouldTrackDataVolume() {
			vmiStatus.Phase = virtv2.ImageReady
			vmiStatus.Progress = ""
		}
		vmiStatus.DownloadSpeed = virtv2.ImageStatusSpeed{}
		finalReport, err := importer.GetFinalImporterReport(state.Pod)
		if err != nil {
			opts.Log.Error(err, "parsing final report", "vmi.name", state.VMI.Current().Name)
		}
		if finalReport != nil {
			vmiStatus.Size.Stored = finalReport.StoredSize()
			vmiStatus.Size.StoredBytes = strconv.FormatUint(finalReport.StoredSizeBytes, 10)
			vmiStatus.Size.Unpacked = finalReport.UnpackedSize()
			vmiStatus.Size.UnpackedBytes = strconv.FormatUint(finalReport.UnpackedSizeBytes, 10)
		}
		// Set target image name the same way as for the importer Pod.
		vmiStatus.Target.RegistryURL = cc.PrepareDVCREndpointFromVMI(state.VMI.Current(), r.dvcrSettings)

	case state.ShouldTrackDataVolume() && state.IsDataVolumeInProgress():
		// Set phase from DataVolume resource.
		vmiStatus.Phase = MapDataVolumePhaseToVMIPhase(state.DV.Status.Phase)

		// Download speed is not available from DataVolume.
		vmiStatus.DownloadSpeed = virtv2.ImageStatusSpeed{}

		// Copy progress from DataVolume.
		// map 0-100% to 0-50%.
		progressPct := string(state.DV.Status.Progress)
		if progressPct != "N/A" {
			if state.ShouldTrackImporterPod() {
				progressPct = common.ScalePercentage(progressPct, 50.0, 100.0)
			}
			vmiStatus.Progress = progressPct
		}

		// Copy capacity from PVC.
		if state.PVC != nil {
			if state.PVC.Status.Phase == corev1.ClaimBound {
				vmiStatus.Capacity = util.GetPointer(state.PVC.Status.Capacity[corev1.ResourceStorage]).String()
			}
		}

	case state.ShouldTrackDataVolume() && state.IsDataVolumeComplete():
		opts.Recorder.Event(state.VMI.Current(), corev1.EventTypeNormal, "ImportSucceededToPVC", "Import Successful")
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

// removeDVFinalizers removes protective finalizers on DataVolume, PersistentVolumeClaim and PersistentVolume dependencies.
func (r *VMIReconciler) removeDVFinalizers(ctx context.Context, state *VMIReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
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

// initImporterPodName creates new name and update it in the annotation.
func (r *VMIReconciler) initImporterPodName(vmi *virtv2.VirtualMachineImage) {
	anno := vmi.GetAnnotations()
	if anno == nil {
		anno = make(map[string]string)
	}

	anno[cc.AnnImportPodName] = fmt.Sprintf("%s-%s", common.ImporterPodNamePrefix, vmi.GetName())
	// Generate name for secret with certs from caBundle.
	if vmiutil.HasCABundle(vmi) {
		anno[cc.AnnCABundleConfigMap] = fmt.Sprintf("%s-ca", vmi.Name)
	}

	vmi.Annotations = anno
}

func (r *VMIReconciler) startImporterPod(ctx context.Context, vmi *virtv2.VirtualMachineImage, opts two_phase_reconciler.ReconcilerOptions) error {
	opts.Log.V(1).Info("Creating importer POD for VMI", "vmi.Name", vmi.Name)

	importerSettings, err := r.createImporterSettings(vmi)
	if err != nil {
		return err
	}

	podSettings := r.createImporterPodSettings(vmi)

	caBundleSettings := importer.NewCABundleSettings(vmiutil.GetCABundle(vmi), vmi.Annotations[cc.AnnCABundleConfigMap])

	imp := importer.NewImporter(podSettings, importerSettings, caBundleSettings)
	pod, err := imp.CreatePod(ctx, opts.Client)
	// Check if pod has failed and, in that case, record an event with the error
	if podErr := cc.HandleFailedPod(err, vmi.Annotations[cc.AnnImportPodName], vmi, opts.Recorder, opts.Client); podErr != nil {
		return podErr
	}

	opts.Log.V(1).Info("Created importer POD", "pod.Name", pod.Name)

	if caBundleSettings != nil {
		if err := imp.EnsureCABundleConfigMap(ctx, opts.Client, pod); err != nil {
			return fmt.Errorf("create ConfigMap with certs from caBundle: %w", err)
		}
		opts.Log.V(1).Info("Created ConfigMap with caBundle", "cm.Name", caBundleSettings.ConfigMapName)
	}

	// TODO add finalizer.
	// // If importing from image stream, add finalizer. Note we don't watch the importer pod in this case,
	// // so to prevent a deadlock we add finalizer only if the pod is not retained after completion.
	// if cc.IsImageStream(pvc) && pvc.GetAnnotations()[cc.AnnPodRetainAfterCompletion] != "true" {
	//	cc.AddFinalizer(pvc, importPodImageStreamFinalizer)
	//	if err := r.updatePVC(pvc, r.log); err != nil {
	//		return err
	//	}
	// }

	return nil
}

func (r *VMIReconciler) createImporterPodSettings(vmi *virtv2.VirtualMachineImage) *importer.PodSettings {
	return &importer.PodSettings{
		Name:            vmi.Annotations[cc.AnnImportPodName],
		Image:           r.image,
		PullPolicy:      r.pullPolicy,
		Namespace:       vmi.GetNamespace(),
		OwnerReference:  vmiutil.MakeOwnerReference(vmi),
		ControllerName:  cvmiControllerName,
		InstallerLabels: map[string]string{},
	}
}

// createImporterSettings fills settings for the registry-importer binary.
func (r *VMIReconciler) createImporterSettings(vmi *virtv2.VirtualMachineImage) (*importer.Settings, error) {
	settings := &importer.Settings{
		Verbose: r.verbose,
		Source:  cc.GetSource(vmi.Spec.DataSource),
	}

	switch settings.Source {
	case cc.SourceHTTP:
		if http := vmi.Spec.DataSource.HTTP; http != nil {
			importer.UpdateHTTPSettings(settings, http)
		}
	case cc.SourceNone:
	default:
		return nil, fmt.Errorf("unknown settings source: %s", settings.Source)
	}

	// Set DVCR settings.
	importer.UpdateDVCRSettings(settings, r.dvcrSettings, cc.PrepareDVCREndpointFromVMI(vmi, r.dvcrSettings))

	// TODO Update proxy settings.

	return settings, nil
}

// initImporterPodName creates new name and update it in the annotation.
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
	// Generate name for secret with certs from caBundle.
	if vmiutil.HasCABundle(vmi) {
		anno[cc.AnnCABundleConfigMap] = strings.ShortenString(fmt.Sprintf("vmi-ca-%s", vmi.GetName()), kvalidation.DNS1123SubdomainMaxLength)
	}

	vmi.Annotations = anno
}

func (r *VMIReconciler) createDataVolume(ctx context.Context, vmi *virtv2.VirtualMachineImage, pvcSize string, opts two_phase_reconciler.ReconcilerOptions) error {
	dvName := types.NamespacedName{Name: vmi.GetAnnotations()[cc.AnnVMIDataVolume], Namespace: vmi.GetNamespace()}
	dvBuilder := kvbuilder.NewDV(dvName)
	err := kvbuilder.ApplyVirtualMachineImageSpec(dvBuilder, vmi, pvcSize)
	if err != nil {
		return fmt.Errorf("apply VMI spec to DataVolume: %w", err)
	}
	dv := dvBuilder.GetResource()

	if err := opts.Client.Create(ctx, dv); err != nil {
		opts.Log.V(2).Info("Error create new DV spec", "dv.spec", dv.Spec)
		return fmt.Errorf("create DataVolume/%s for VMI/%s: %w", dv.GetName(), vmi.GetName(), err)
	}
	opts.Log.Info("Created new DV", "dv.name", dv.GetName())
	opts.Log.V(2).Info("Created new DV spec", "dv.spec", dv.Spec)
	return nil
}
