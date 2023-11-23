package controller

import (
	"context"
	"fmt"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
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
	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/monitoring"
	"github.com/deckhouse/virtualization-controller/pkg/controller/uploader"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmattachee"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
)

type CVMIReconciler struct {
	*vmattachee.AttacheeReconciler[*virtv2.ClusterVirtualMachineImage, virtv2.ClusterVirtualMachineImageStatus]

	importerImage   string
	uploaderImage   string
	verbose         string
	pullPolicy      string
	installerLabels map[string]string
	namespace       string
	dvcrSettings    *dvcr.Settings
}

func NewCVMIReconciler(importerImage, uploaderImage, verbose, pullPolicy, namespace string, dvcrSettings *dvcr.Settings) *CVMIReconciler {
	return &CVMIReconciler{
		importerImage: importerImage,
		uploaderImage: uploaderImage,
		verbose:       verbose,
		pullPolicy:    pullPolicy,
		namespace:     namespace,
		dvcrSettings:  dvcrSettings,
		AttacheeReconciler: vmattachee.NewAttacheeReconciler[
			*virtv2.ClusterVirtualMachineImage,
			virtv2.ClusterVirtualMachineImageStatus,
		]("cvmi", false),
	}
}

func (r *CVMIReconciler) SetupController(ctx context.Context, mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.ClusterVirtualMachineImage{}),
		&handler.EnqueueRequestForObject{},
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool { return true },
		},
	); err != nil {
		return err
	}

	return r.AttacheeReconciler.SetupController(ctx, mgr, ctr)
}

// Sync creates and deletes importer Pod depending on CVMI status.
func (r *CVMIReconciler) Sync(ctx context.Context, _ reconcile.Request, state *CVMIReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	opts.Log.Info("Reconcile required for CVMI", "cvmi.name", state.CVMI.Current().Name, "cvmi.phase", state.CVMI.Current().Status.Phase)

	if r.AttacheeReconciler.Sync(ctx, state.AttacheeState, opts) {
		return nil
	}

	// Change the world depending on states of CVMI and Pod.
	switch {
	case state.IsDeletion():
		return r.removeFinalizers(ctx, state, opts)
	case !state.IsProtected():
		if err := r.verifyDataSourceRefs(ctx, opts.Client, state); err != nil {
			return err
		}
		// Set protective finalizer atomically on a verified resource.
		if controllerutil.AddFinalizer(state.CVMI.Changed(), virtv2.FinalizerCVMICleanup) {
			state.SetReconcilerResult(&reconcile.Result{Requeue: true})
			return nil
		}
	case !r.isInited(state.CVMI.Changed(), state):
		opts.Log.V(1).Info("New CVMI observed, update annotations with Pod name and namespace")
		// TODO(i.mikh) This algorithm is from CDI: put annotation on fresh CVMI and run Pod on next call to reconcile. Is it ok?
		r.initPodName(state)
		// Update annotations and status and restart reconcile to create an importer/uploader Pod.
		state.SetReconcilerResult(&reconcile.Result{Requeue: true})
		return nil
	case r.isImportComplete(state):
		// Note: state.ShouldReconcile was positive, so state.Pod is not nil and should be deleted.
		// Delete sub recourses (Pods, Services, Secrets) when CVMI is marked as ready and stop the reconcile process.
		if cc.ShouldCleanupSubResources(state.CVMI.Current()) {
			opts.Log.V(1).Info("Import done, cleanup")
			if err := r.removeFinalizers(ctx, state, opts); err != nil {
				return err
			}
			return r.cleanup(ctx, state.CVMI.Changed(), opts.Client, state)
		}
		return nil
	case r.canStart(state):
		// Create Pod using name and namespace from annotation.
		opts.Log.V(1).Info("Pod for CVMI not found, create new one")

		if err := r.startPod(ctx, state, opts); err != nil {
			return err
		}

		// Requeue to wait until Pod become Running.
		state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})
		return nil
	case r.isInPending(state.CVMI.Current(), state), r.isInProgress(state.CVMI.Current(), state):
		// Import is in progress, force a re-reconcile in 2 seconds to update status.
		opts.Log.V(2).Info("Requeue: CVMI import is in progress", "cvmi.name", state.CVMI.Current().Name)
		if err := r.ensurePodFinalizers(ctx, state, opts); err != nil {
			return err
		}
		state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})
		return nil
	}

	// Report unexpected state.
	details := fmt.Sprintf("cvmi.Status.Phase='%s'", state.CVMI.Current().Status.Phase)
	if state.Pod != nil {
		details += fmt.Sprintf(" pod.Name='%s' pod.Status.Phase='%s'", state.Pod.Name, state.Pod.Status.Phase)
	}
	opts.Recorder.Event(state.CVMI.Current(), corev1.EventTypeWarning, virtv2.ReasonErrUnknownState, fmt.Sprintf("CVMI has unexpected state, recreate it to start import again. %s", details))

	return nil
}

func (r *CVMIReconciler) UpdateStatus(ctx context.Context, _ reconcile.Request, state *CVMIReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	opts.Log.V(2).Info("Update CVMI status")

	// Record event if Pod has error.
	// TODO set Failed status if Pod restarts are greater than some threshold?
	if state.Pod != nil && len(state.Pod.Status.ContainerStatuses) > 0 {
		if state.Pod.Status.ContainerStatuses[0].LastTerminationState.Terminated != nil &&
			state.Pod.Status.ContainerStatuses[0].LastTerminationState.Terminated.ExitCode > 0 {
			opts.Recorder.Event(state.CVMI.Current(), corev1.EventTypeWarning, virtv2.ReasonErrImportFailed, fmt.Sprintf("pod phase '%s', message '%s'", state.Pod.Status.Phase, state.Pod.Status.ContainerStatuses[0].LastTerminationState.Terminated.Message))
		}
	}

	cvmiStatus := state.CVMI.Current().Status.DeepCopy()

	// Set target image name the same way as for the importer/uploader Pod.
	dvcrDestImageName := r.dvcrSettings.RegistryImageForCVMI(state.CVMI.Current().Name)
	cvmiStatus.Target.RegistryURL = dvcrDestImageName

	if cvmiStatus.Phase != virtv2.ImageReady {
		cvmiStatus.ImportDuration = time.Since(state.CVMI.Current().CreationTimestamp.Time).Truncate(time.Second).String()
	}

	switch {
	case !r.isInited(state.CVMI.Current(), state), state.CVMI.Current().Status.Phase == "":
		cvmiStatus.Phase = virtv2.ImagePending
		if err := r.verifyDataSourceRefs(ctx, opts.Client, state); err != nil {
			cvmiStatus.FailureReason = FailureReasonCannotBeProcessed
			cvmiStatus.FailureMessage = fmt.Sprintf("DataSource is invalid. %s", err)
		}
	case r.isReady(state.CVMI.Current(), state):
		break
	case r.isInProgress(state.CVMI.Current(), state):
		// Set CVMI status to Provisioning and copy progress metrics from importer/uploader Pod.
		opts.Log.V(2).Info("Fetch progress", "cvmi.name", state.CVMI.Current().Name)
		cvmiStatus.Phase = virtv2.ImageProvisioning
		t := state.CVMI.Current().Spec.DataSource.Type
		if t == virtv2.DataSourceTypeUpload &&
			cvmiStatus.UploadCommand == "" &&
			state.Service != nil &&
			len(state.Service.Spec.Ports) > 0 {
			cvmiStatus.UploadCommand = fmt.Sprintf(
				"curl -X POST --data-binary @example.iso http://%s:%d/v1beta1/upload",
				state.Service.Spec.ClusterIP,
				state.Service.Spec.Ports[0].Port,
			)
		}
		var progress *monitoring.ImportProgress
		if t != virtv2.DataSourceTypeClusterVirtualMachineImage &&
			t != virtv2.DataSourceTypeVirtualMachineImage {
			progress, err := monitoring.GetImportProgressFromPod(string(state.CVMI.Current().GetUID()), state.Pod)
			if err != nil {
				opts.Recorder.Event(state.CVMI.Current(), corev1.EventTypeWarning, virtv2.ReasonErrGetProgressFailed, "Error fetching progress metrics from Pod "+err.Error())
				return err
			}
			if progress != nil {
				opts.Log.V(2).Info("Got progress", "cvmi.name", state.CVMI.Current().Name, "progress", progress.Progress(), "speed", progress.AvgSpeed(), "progress.raw", progress.ProgressRaw(), "speed.raw", progress.AvgSpeedRaw())
				cvmiStatus.Progress = progress.Progress()
				cvmiStatus.DownloadSpeed.Avg = progress.AvgSpeed()
				cvmiStatus.DownloadSpeed.AvgBytes = strconv.FormatUint(progress.AvgSpeedRaw(), 10)
				cvmiStatus.DownloadSpeed.Current = progress.CurSpeed()
				cvmiStatus.DownloadSpeed.CurrentBytes = strconv.FormatUint(progress.CurSpeedRaw(), 10)
			}
		}
		// Set CVMI phase.
		if state.CVMI.Current().Spec.DataSource.Type == virtv2.DataSourceTypeUpload && (progress == nil || progress.ProgressRaw() == 0) {
			cvmiStatus.Phase = virtv2.ImageWaitForUserUpload
		} else {
			cvmiStatus.Phase = virtv2.ImageProvisioning
		}
	case r.isImportComplete(state):
		// Set CVMI status to Ready and update image size from final report of the importer/uploader Pod.
		opts.Recorder.Event(state.CVMI.Current(), corev1.EventTypeNormal, virtv2.ReasonImportSucceeded, "Import Successful")
		opts.Log.V(1).Info("Import completed successfully")
		cvmiStatus.Phase = virtv2.ImageReady
		cvmiStatus.Progress = "100%"
		// Cleanup.
		cvmiStatus.DownloadSpeed.Current = ""
		cvmiStatus.DownloadSpeed.CurrentBytes = ""
		finalReport, err := monitoring.GetFinalReportFromPod(state.Pod)
		if err != nil {
			opts.Log.Error(err, "parsing final report", "cvmi.name", state.CVMI.Current().Name)
		}
		if finalReport != nil {
			cvmiStatus.DownloadSpeed.Avg = finalReport.GetAverageSpeed()
			cvmiStatus.DownloadSpeed.AvgBytes = strconv.FormatUint(finalReport.GetAverageSpeedRaw(), 10)
			cvmiStatus.Size.Stored = finalReport.StoredSize()
			cvmiStatus.Size.StoredBytes = strconv.FormatUint(finalReport.StoredSizeBytes, 10)
			cvmiStatus.Size.Unpacked = finalReport.UnpackedSize()
			cvmiStatus.Size.UnpackedBytes = strconv.FormatUint(finalReport.UnpackedSizeBytes, 10)

			switch state.CVMI.Current().Spec.DataSource.Type {
			case virtv2.DataSourceTypeClusterVirtualMachineImage:
				cvmiStatus.Format = state.DVCRDataSource.CVMI.Status.Format
			case virtv2.DataSourceTypeVirtualMachineImage:
				cvmiStatus.Format = state.DVCRDataSource.VMI.Status.Format
			default:
				cvmiStatus.Format = finalReport.Format
			}
		}
	}

	state.CVMI.Changed().Status = *cvmiStatus

	return nil
}

func (r *CVMIReconciler) verifyDataSourceRefs(ctx context.Context, client client.Client, state *CVMIReconcilerState) error {
	cvmi := state.CVMI.Current()
	switch cvmi.Spec.DataSource.Type {
	case virtv2.DataSourceTypeClusterVirtualMachineImage, virtv2.DataSourceTypeVirtualMachineImage:
		if err := VerifyDVCRDataSources(cvmi.Spec.DataSource, state.DVCRDataSource); err != nil {
			return err
		}
	case virtv2.DataSourceTypeContainerImage:
		if cvmi.Spec.DataSource.ContainerImage != nil {
			return fmt.Errorf("dataSource '%s' specified without related 'containerImage' section", cvmi.Spec.DataSource.Type)
		}
		if cvmi.Spec.DataSource.ContainerImage.ImagePullSecret.Name != "" {
			ns := cvmi.Spec.DataSource.ContainerImage.ImagePullSecret.Namespace
			if ns == "" {
				ns = cvmi.GetNamespace()
			}
			secretName := types.NamespacedName{
				Namespace: ns,
				Name:      cvmi.Spec.DataSource.ContainerImage.ImagePullSecret.Name,
			}
			srcSecret, err := helper.FetchObject[*corev1.Secret](ctx, secretName, client, &corev1.Secret{})
			if err != nil || srcSecret == nil {
				return fmt.Errorf("containerImage.imagePullSecret %s not found", secretName.String())
			}
		}
	}
	return nil
}

func (r *CVMIReconciler) isInited(cvmi *virtv2.ClusterVirtualMachineImage, state *CVMIReconcilerState) bool {
	switch cvmi.Spec.DataSource.Type {
	case virtv2.DataSourceTypeUpload:
		return state.HasUploaderAnno()
	default:
		return state.HasImporterAnno()
	}
}

func (r *CVMIReconciler) canStart(state *CVMIReconcilerState) bool {
	return state.Pod == nil
}

func (r *CVMIReconciler) isInProgress(cvmi *virtv2.ClusterVirtualMachineImage, state *CVMIReconcilerState) bool {
	if state.Pod == nil {
		return false
	}

	return r.isInited(cvmi, state) && state.Pod.Status.Phase == corev1.PodRunning
}

func (r *CVMIReconciler) isInPending(cvmi *virtv2.ClusterVirtualMachineImage, state *CVMIReconcilerState) bool {
	if state.Pod == nil {
		return false
	}

	return r.isInited(cvmi, state) && state.Pod.Status.Phase == corev1.PodPending
}

func (r *CVMIReconciler) isImportComplete(state *CVMIReconcilerState) bool {
	if state.CVMI.IsEmpty() {
		return false
	}

	if !r.isInited(state.CVMI.Current(), state) {
		return false
	}

	return state.Pod != nil && cc.IsPodComplete(state.Pod)
}

func (r *CVMIReconciler) isReady(cvmi *virtv2.ClusterVirtualMachineImage, state *CVMIReconcilerState) bool {
	if state.CVMI.IsEmpty() {
		return false
	}

	if !r.isInited(cvmi, state) {
		return false
	}

	return state.CVMI.Current().Status.Phase == virtv2.ImageReady
}

func (r *CVMIReconciler) cleanup(ctx context.Context, cvmi *virtv2.ClusterVirtualMachineImage, client client.Client, state *CVMIReconcilerState) error {
	switch cvmi.Spec.DataSource.Type {
	case virtv2.DataSourceTypeUpload:
		if err := uploader.CleanupService(ctx, client, state.Service); err != nil {
			return err
		}

		return uploader.CleanupPod(ctx, client, state.Pod)
	default:
		return importer.CleanupPod(ctx, client, state.Pod)
	}
}

func (r *CVMIReconciler) startPod(
	ctx context.Context,
	state *CVMIReconcilerState,
	opts two_phase_reconciler.ReconcilerOptions,
) error {
	switch state.CVMI.Current().Spec.DataSource.Type {
	case virtv2.DataSourceTypeUpload:
		if err := r.startUploaderPod(ctx, state, opts); err != nil {
			return err
		}

		if err := r.startUploaderService(ctx, state, opts); err != nil {
			return err
		}
	default:
		if err := r.startImporterPod(ctx, state, opts); err != nil {
			return err
		}
	}

	return nil
}

// initCVMIPodName saves the Pod name in the annotation.
func (r *CVMIReconciler) initPodName(state *CVMIReconcilerState) {
	cvmi := state.CVMI.Changed()

	switch cvmi.Spec.DataSource.Type {
	case virtv2.DataSourceTypeUpload:
		uploaderPod := state.Supplements.UploaderPod()
		cc.AddAnnotation(cvmi, cc.AnnUploaderNamespace, uploaderPod.Namespace)
		cc.AddAnnotation(cvmi, cc.AnnUploadPodName, uploaderPod.Name)
	default:
		importerPod := state.Supplements.ImporterPod()
		cc.AddAnnotation(cvmi, cc.AnnImporterNamespace, importerPod.Namespace)
		cc.AddAnnotation(cvmi, cc.AnnImportPodName, importerPod.Name)
	}
}

// ensurePodFinalizers adds protective finalizers on importer/uploader Pod and Service dependencies.
func (r *CVMIReconciler) ensurePodFinalizers(ctx context.Context, state *CVMIReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
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

// removeFinalizers removes protective finalizers on Pod and Service dependencies.
func (r *CVMIReconciler) removeFinalizers(ctx context.Context, state *CVMIReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
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

	controllerutil.RemoveFinalizer(state.CVMI.Changed(), virtv2.FinalizerCVMICleanup)

	return nil
}
