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

	cvmiutil "github.com/deckhouse/virtualization-controller/pkg/common/cvmi"
	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/monitoring"
	"github.com/deckhouse/virtualization-controller/pkg/controller/uploader"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmattachee"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/imageformat"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type CVMIReconciler struct {
	*vmattachee.AttacheeReconciler[*virtv2.ClusterVirtualImage, virtv2.ClusterVirtualImageStatus]

	importerImage string
	uploaderImage string
	verbose       string
	pullPolicy    string
	dvcrSettings  *dvcr.Settings
}

func NewCVMIReconciler(importerImage, uploaderImage, verbose, pullPolicy string, dvcrSettings *dvcr.Settings) *CVMIReconciler {
	return &CVMIReconciler{
		importerImage: importerImage,
		uploaderImage: uploaderImage,
		verbose:       verbose,
		pullPolicy:    pullPolicy,
		dvcrSettings:  dvcrSettings,
		AttacheeReconciler: vmattachee.NewAttacheeReconciler[
			*virtv2.ClusterVirtualImage,
			virtv2.ClusterVirtualImageStatus,
		](),
	}
}

func (r *CVMIReconciler) SetupController(ctx context.Context, mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.ClusterVirtualImage{}),
		&handler.EnqueueRequestForObject{},
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool { return true },
		},
	); err != nil {
		return err
	}

	return r.AttacheeReconciler.SetupController(mgr, ctr, r)
}

// Sync creates and deletes importer Pod depending on CVMI status.
func (r *CVMIReconciler) Sync(ctx context.Context, _ reconcile.Request, state *CVMIReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	opts.Log.Info("Reconcile required for CVI", "cvi.name", state.CVMI.Current().Name, "cvi.phase", state.CVMI.Current().Status.Phase)

	if r.AttacheeReconciler.Sync(ctx, state.AttacheeState, opts) {
		return nil
	}

	// Change the world depending on states of CVMI and Pod.
	switch {
	case state.IsDeletion():
		opts.Log.V(1).Info("Delete CVI, remove protective finalizers")
		return r.cleanupOnDeletion(ctx, state, opts)
	case !state.IsProtected():
		if err := r.verifyDataSourceRefs(ctx, opts.Client, state); err != nil {
			return err
		}
		// Set protective finalizer atomically on a verified resource.
		if controllerutil.AddFinalizer(state.CVMI.Changed(), virtv2.FinalizerCVMICleanup) {
			state.SetReconcilerResult(&reconcile.Result{Requeue: true})
			return nil
		}
	case state.IsPodComplete():
		// Note: state.ShouldReconcile was positive, so state.Pod is not nil and should be deleted.
		// Delete sub recourses (Pods, Services, Secrets) when CVMI is marked as ready and stop the reconcile process.
		if cc.ShouldCleanupSubResources(state.CVMI.Current()) {
			opts.Log.V(1).Info("Import done, cleanup")
			return r.cleanup(ctx, state.CVMI.Changed(), opts.Client, state)
		}
		return nil
	case state.IsFailed():
		opts.Log.Info("CVI failed: cleanup underlying resources")
		// Delete underlying importer/uploader Pod, Service and DataVolume and stop the reconcile process.
		if cc.ShouldCleanupSubResources(state.CVMI.Current()) {
			return r.cleanup(ctx, state.CVMI.Changed(), opts.Client, state)
		}
		return nil
	case state.CanStartPod():
		// Create Pod using name and namespace from annotation.
		opts.Log.V(1).Info("Pod for CVI not found, create new one")

		if cvmiutil.IsDVCRSource(state.CVMI.Current()) && !state.DVCRDataSource.IsReady() {
			opts.Log.V(1).Info("Wait for the data source to be ready")
			state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})
			return nil
		}

		if err := r.startPod(ctx, state, opts); err != nil {
			return err
		}

		// Requeue to wait until Pod become Running.
		state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})
		return nil
	case state.IsImportInProgress(), state.IsImportInPending():
		// Import is in progress, force a re-reconcile in 2 seconds to update status.
		opts.Log.V(2).Info("Requeue: CVI import is in progress", "cvi.name", state.CVMI.Current().Name)
		if err := r.ensurePodFinalizers(ctx, state, opts); err != nil {
			return err
		}
		state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})
		return nil
	}

	// Report unexpected state.
	details := fmt.Sprintf("cvi.Status.Phase='%s'", state.CVMI.Current().Status.Phase)
	if state.Pod != nil {
		details += fmt.Sprintf(" pod.Name='%s' pod.Status.Phase='%s'", state.Pod.Name, state.Pod.Status.Phase)
	}
	opts.Recorder.Event(state.CVMI.Current(), corev1.EventTypeWarning, virtv2.ReasonErrUnknownState, fmt.Sprintf("CVI has unexpected state, recreate it to start import again. %s", details))

	return nil
}

func (r *CVMIReconciler) UpdateStatus(ctx context.Context, _ reconcile.Request, state *CVMIReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	opts.Log.V(2).Info("Update CVI status")

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

	switch {
	case state.CVMI.Current().Status.Phase == "":
		cvmiStatus.Phase = virtv2.ImagePending
		if err := r.verifyDataSourceRefs(ctx, opts.Client, state); err != nil {
			cvmiStatus.FailureReason = FailureReasonCannotBeProcessed
			cvmiStatus.FailureMessage = fmt.Sprintf("DataSource is invalid. %s", err)
		}
	case state.IsReady(), state.IsFailed():
		break
	case !state.IsPodComplete():
		// Set CVMI status to Provisioning and copy progress metrics from importer/uploader Pod.
		opts.Log.V(2).Info("Fetch progress", "cvi.name", state.CVMI.Current().Name)
		cvmiStatus.Phase = virtv2.ImageProvisioning
		if state.CVMI.Current().Spec.DataSource.Type == virtv2.DataSourceTypeUpload &&
			cvmiStatus.UploadCommand == "" &&
			state.Ingress != nil &&
			state.Ingress.GetAnnotations()[cc.AnnUploadURL] != "" {
			cvmiStatus.UploadCommand = fmt.Sprintf(
				"curl %s -T example.iso",
				state.Ingress.GetAnnotations()[cc.AnnUploadURL],
			)
		}
		var progress *monitoring.ImportProgress
		if !cvmiutil.IsDVCRSource(state.CVMI.Current()) {
			var err error
			progress, err = monitoring.GetImportProgressFromPod(string(state.CVMI.Current().GetUID()), state.Pod)
			if err != nil {
				opts.Recorder.Event(state.CVMI.Current(), corev1.EventTypeWarning, virtv2.ReasonErrGetProgressFailed, "Error fetching progress metrics from Pod "+err.Error())
				return err
			}
			if progress != nil {
				opts.Log.V(2).Info("Got progress", "cvi.name", state.CVMI.Current().Name, "progress", progress.Progress(), "speed", progress.AvgSpeed(), "progress.raw", progress.ProgressRaw(), "speed.raw", progress.AvgSpeedRaw())
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
			if state.Pod != nil && helper.GetAge(state.Pod) > cc.UploaderWaitDuration {
				cvmiStatus.Phase = virtv2.ImageFailed
				cvmiStatus.FailureReason = virtv2.ReasonErrUploaderWaitDurationExpired
				cvmiStatus.FailureMessage = "uploading time expired"
			}
		} else {
			cvmiStatus.Phase = virtv2.ImageProvisioning
		}
	case state.IsPodComplete():
		// Set CVMI status to Ready and update image size from final report of the importer/uploader Pod.
		opts.Recorder.Event(state.CVMI.Current(), corev1.EventTypeNormal, virtv2.ReasonImportSucceeded, "Import Successful")
		opts.Log.V(1).Info("Import completed successfully")
		cvmiStatus.Phase = virtv2.ImageReady
		cvmiStatus.Progress = "100%"
		// Cleanup.
		cvmiStatus.DownloadSpeed.Current = ""
		cvmiStatus.DownloadSpeed.CurrentBytes = ""

		switch {
		case cvmiutil.IsDVCRSource(state.CVMI.Current()):
			cvmiStatus.Format = state.DVCRDataSource.GetFormat()
			cvmiStatus.CDROM = imageformat.IsISO(cvmiStatus.Format)
			cvmiStatus.Size = state.DVCRDataSource.GetSize()
		default:
			finalReport, err := monitoring.GetFinalReportFromPod(state.Pod)
			if err != nil {
				return err
			}

			if finalReport.ErrMessage != "" {
				cvmiStatus.Phase = virtv2.ImageFailed
				cvmiStatus.FailureReason = virtv2.ReasonErrImportFailed
				cvmiStatus.FailureMessage = finalReport.ErrMessage
				opts.Recorder.Event(state.CVMI.Current(), corev1.EventTypeWarning, virtv2.ReasonErrImportFailed, finalReport.ErrMessage)
				break
			}

			cvmiStatus.Format = finalReport.Format
			cvmiStatus.CDROM = imageformat.IsISO(cvmiStatus.Format)
			cvmiStatus.DownloadSpeed.Avg = finalReport.GetAverageSpeed()
			cvmiStatus.DownloadSpeed.AvgBytes = strconv.FormatUint(finalReport.GetAverageSpeedRaw(), 10)
			cvmiStatus.Size.Stored = finalReport.StoredSize()
			cvmiStatus.Size.StoredBytes = strconv.FormatUint(finalReport.StoredSizeBytes, 10)
			cvmiStatus.Size.Unpacked = finalReport.UnpackedSize()
			cvmiStatus.Size.UnpackedBytes = strconv.FormatUint(finalReport.UnpackedSizeBytes, 10)
		}
	}

	state.CVMI.Changed().Status = *cvmiStatus

	return nil
}

func (r *CVMIReconciler) verifyDataSourceRefs(ctx context.Context, client client.Client, state *CVMIReconcilerState) error {
	cvmi := state.CVMI.Current()
	switch cvmi.Spec.DataSource.Type {
	case virtv2.DataSourceTypeObjectRef:
		if err := state.DVCRDataSource.Validate(); err != nil {
			return err
		}
	case virtv2.DataSourceTypeContainerImage:
		if cvmi.Spec.DataSource.ContainerImage == nil {
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

func (r *CVMIReconciler) cleanup(ctx context.Context, cvmi *virtv2.ClusterVirtualImage, client client.Client, state *CVMIReconcilerState) error {
	switch cvmi.Spec.DataSource.Type {
	case virtv2.DataSourceTypeUpload:
		if state.Ingress != nil {
			if err := uploader.CleanupIngress(ctx, client, state.Ingress); err != nil {
				return err
			}
		}
		if state.Service != nil {
			if err := uploader.CleanupService(ctx, client, state.Service); err != nil {
				return err
			}
		}
		if state.Pod != nil {
			if err := uploader.CleanupPod(ctx, client, state.Pod); err != nil {
				return err
			}
		}
	default:
		if state.Pod != nil {
			if err := importer.CleanupPod(ctx, client, state.Pod); err != nil {
				return err
			}
		}
	}
	return nil
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
		if err := r.startUploaderIngress(ctx, state, opts); err != nil {
			return err
		}
	default:
		if err := r.startImporterPod(ctx, state, opts); err != nil {
			return err
		}
	}

	return nil
}

// ensurePodFinalizers adds protective finalizers on importer/uploader Pod and Service dependencies.
func (r *CVMIReconciler) ensurePodFinalizers(ctx context.Context, state *CVMIReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	if state.Pod != nil && controllerutil.AddFinalizer(state.Pod, virtv2.FinalizerPodProtection) {
		if err := opts.Client.Update(ctx, state.Pod); err != nil {
			return fmt.Errorf("error setting finalizer on a Pod %q: %w", state.Pod.Name, err)
		}
	}
	if state.Service != nil && controllerutil.AddFinalizer(state.Service, virtv2.FinalizerServiceProtection) {
		if err := opts.Client.Update(ctx, state.Service); err != nil {
			return fmt.Errorf("error setting finalizer on a Service %q: %w", state.Service.Name, err)
		}
	}
	if state.Ingress != nil && controllerutil.AddFinalizer(state.Ingress, virtv2.FinalizerIngressProtection) {
		if err := opts.Client.Update(ctx, state.Ingress); err != nil {
			return fmt.Errorf("error setting finalizer on a Ingress %q: %w", state.Ingress.Name, err)
		}
	}

	return nil
}

func (r *CVMIReconciler) ShouldDeleteChildResources(state *CVMIReconcilerState) bool {
	return state.Pod != nil || state.Service != nil || state.Ingress != nil
}

func (r *CVMIReconciler) cleanupOnDeletion(ctx context.Context, state *CVMIReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	if r.ShouldDeleteChildResources(state) {
		if err := r.cleanup(ctx, state.CVMI.Current(), opts.Client, state); err != nil {
			return err
		}
		state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})
		return nil
	}
	controllerutil.RemoveFinalizer(state.CVMI.Changed(), virtv2.FinalizerCVMICleanup)
	return nil
}

func (r *CVMIReconciler) FilterAttachedVM(vm *virtv2.VirtualMachine) bool {
	for _, bda := range vm.Status.BlockDeviceRefs {
		if bda.Kind == virtv2.ClusterImageDevice {
			return true
		}
	}

	return false
}

func (r *CVMIReconciler) EnqueueFromAttachedVM(vm *virtv2.VirtualMachine) []reconcile.Request {
	var requests []reconcile.Request

	for _, bda := range vm.Status.BlockDeviceRefs {
		if bda.Kind != virtv2.ClusterImageDevice {
			continue
		}

		requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{
			Name: bda.Name,
		}})
	}

	return requests
}
