package controller

import (
	"context"
	"fmt"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	virtv2alpha1 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	"github.com/deckhouse/virtualization-controller/pkg/common"
	cvmiutil "github.com/deckhouse/virtualization-controller/pkg/common/cvmi"
	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/monitoring"
	"github.com/deckhouse/virtualization-controller/pkg/controller/uploader"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmattachee"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
)

type CVMIReconciler struct {
	*vmattachee.AttacheeReconciler[*virtv2alpha1.ClusterVirtualMachineImage, virtv2alpha1.ClusterVirtualMachineImageStatus]

	importerImage   string
	uploaderImage   string
	verbose         string
	pullPolicy      string
	installerLabels map[string]string
	namespace       string
	dvcrSettings    *cc.DVCRSettings
}

func NewCVMIReconciler(importerImage, uploaderImage, verbose, pullPolicy, namespace string, dvcrSettings *cc.DVCRSettings) *CVMIReconciler {
	return &CVMIReconciler{
		importerImage: importerImage,
		uploaderImage: uploaderImage,
		verbose:       verbose,
		pullPolicy:    pullPolicy,
		namespace:     namespace,
		dvcrSettings:  dvcrSettings,
		AttacheeReconciler: vmattachee.NewAttacheeReconciler[
			*virtv2alpha1.ClusterVirtualMachineImage,
			virtv2alpha1.ClusterVirtualMachineImageStatus,
		]("cvmi", false),
	}
}

func (r *CVMIReconciler) SetupController(ctx context.Context, mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2alpha1.ClusterVirtualMachineImage{}),
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
		if state.Pod != nil {
			// Delete existing Pod and Service if CVMI is deleted.
			opts.Log.V(1).Info("CVMI is being deleted, cleanup", "pod.Name", state.Pod.Name)
			if err := r.cleanup(ctx, state.CVMI.Changed(), opts.Client, state); err != nil {
				return err
			}
		}
		return nil
	case !r.isInited(state.CVMI.Changed(), state):
		opts.Log.V(1).Info("New CVMI observed, update annotations with Pod name and namespace")
		// TODO(i.mikh) This algorithm is from CDI: put annotation on fresh CVMI and run Pod on next call to reconcile. Is it ok?
		r.init(state.CVMI.Changed())
		// Update annotations and status and restart reconcile to create an importer/uploader Pod.
		state.SetReconcilerResult(&reconcile.Result{Requeue: true})
		return nil
	case r.isReady(state.CVMI.Current(), state):
		// Note: state.ShouldReconcile was positive, so state.Pod is not nil and should be deleted.
		// Delete sub recourses (Pods and Services) when CVMI is marked as ready and stop the reconcile process.
		if cc.ShouldCleanupSubResources(state.CVMI.Current()) {
			opts.Log.V(1).Info("Import done, cleanup")
			return r.cleanup(ctx, state.CVMI.Changed(), opts.Client, state)
		}
	case r.canStart(state.CVMI.Current(), state):
		// Create Pod using name and namespace from annotation.
		opts.Log.V(1).Info("Pod for CVMI not found, create new one")

		if err := r.start(ctx, state.CVMI.Current(), opts); err != nil {
			return err
		}

		// Requeue to wait until Pod become Running.
		state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})
		return nil
	case r.isInPending(state.CVMI.Current(), state), r.isInProgress(state.CVMI.Current(), state):
		// Import is in progress, force a re-reconcile in 2 seconds to update status.
		opts.Log.V(2).Info("Requeue: CVMI import is in progress", "cvmi.name", state.CVMI.Current().Name)
		state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})
		return nil
	}

	// Report unexpected state.
	details := fmt.Sprintf("cvmi.Status.Phase='%s'", state.CVMI.Current().Status.Phase)
	if state.Pod != nil {
		details += fmt.Sprintf(" pod.Name='%s' pod.Status.Phase='%s'", state.Pod.Name, state.Pod.Status.Phase)
	}
	opts.Recorder.Event(state.CVMI.Current(), corev1.EventTypeWarning, "ErrUnknownState", fmt.Sprintf("CVMI has unexpected state, recreate it to start import again. %s", details))

	return nil
}

func (r *CVMIReconciler) UpdateStatus(_ context.Context, _ reconcile.Request, state *CVMIReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	opts.Log.V(2).Info("Update CVMI status")

	// Record event if Pod has error.
	// TODO set Failed status if Pod restarts are greater than some threshold?
	if state.Pod != nil && len(state.Pod.Status.ContainerStatuses) > 0 {
		if state.Pod.Status.ContainerStatuses[0].LastTerminationState.Terminated != nil &&
			state.Pod.Status.ContainerStatuses[0].LastTerminationState.Terminated.ExitCode > 0 {
			opts.Recorder.Event(state.CVMI.Current(), corev1.EventTypeWarning, "ErrImportFailed", fmt.Sprintf("pod phase '%s', message '%s'", state.Pod.Status.Phase, state.Pod.Status.ContainerStatuses[0].LastTerminationState.Terminated.Message))
		}
	}

	cvmiStatus := state.CVMI.Current().Status.DeepCopy()

	// Set target image name the same way as for the importer/uploader Pod.
	cvmiStatus.Target.RegistryURL = cc.PrepareDVCREndpointFromCVMI(state.CVMI.Current(), r.dvcrSettings)

	switch {
	case !r.isInited(state.CVMI.Current(), state), state.CVMI.Current().Status.Phase == "":
		cvmiStatus.Phase = virtv2alpha1.ImagePending
	case r.isReady(state.CVMI.Current(), state):
		break
	case r.isInProgress(state.CVMI.Current(), state):
		// Set CVMI status to Provisioning and copy progress metrics from importer/uploader Pod.
		opts.Log.V(2).Info("Fetch progress", "cvmi.name", state.CVMI.Current().Name)
		cvmiStatus.Phase = virtv2alpha1.ImageProvisioning

		if state.CVMI.Current().Spec.DataSource.Type == virtv2alpha1.DataSourceTypeUpload &&
			cvmiStatus.UploadCommand == "" &&
			state.Service != nil &&
			len(state.Service.Spec.Ports) > 0 {
			cvmiStatus.UploadCommand = fmt.Sprintf(
				"curl -X POST --data-binary @example.iso http://%s:%d/v1beta1/upload",
				state.Service.Spec.ClusterIP,
				state.Service.Spec.Ports[0].Port,
			)
		}

		progress, err := monitoring.GetImportProgressFromPod(string(state.CVMI.Current().GetUID()), state.Pod)
		if err != nil {
			opts.Recorder.Event(state.CVMI.Current(), corev1.EventTypeWarning, "ErrGetProgressFailed", "Error fetching progress metrics from Pod "+err.Error())
			return err
		}
		if progress != nil {
			opts.Log.V(2).Info("Got progress", "cvmi.name", state.CVMI.Current().Name, "progress", progress.Progress(), "speed", progress.AvgSpeed(), "progress.raw", progress.ProgressRaw(), "speed.raw", progress.AvgSpeedRaw())
			cvmiStatus.Progress = progress.Progress()
			cvmiStatus.DownloadSpeed.Avg = progress.AvgSpeed()
			cvmiStatus.DownloadSpeed.AvgBytes = strconv.FormatUint(progress.AvgSpeedRaw(), 10)
			cvmiStatus.DownloadSpeed.Current = progress.CurrentSpeed()
			cvmiStatus.DownloadSpeed.CurrentBytes = strconv.FormatUint(progress.CurrentSpeedRaw(), 10)
		}
	case r.isImportComplete(state):
		// Set CVMI status to Ready and update image size from final report of the importer/uploader Pod.
		opts.Recorder.Event(state.CVMI.Current(), corev1.EventTypeNormal, "ImportSucceeded", "Import Successful")
		opts.Log.V(1).Info("Import completed successfully")
		cvmiStatus.Phase = virtv2alpha1.ImageReady
		// Cleanup.
		cvmiStatus.Progress = ""
		cvmiStatus.DownloadSpeed = virtv2alpha1.ImageStatusSpeed{}
		finalReport, err := monitoring.GetFinalReportFromPod(state.Pod)
		if err != nil {
			opts.Log.Error(err, "parsing final report", "cvmi.name", state.CVMI.Current().Name)
		}
		if finalReport != nil {
			cvmiStatus.Size.Stored = finalReport.StoredSize()
			cvmiStatus.Size.StoredBytes = strconv.FormatUint(finalReport.StoredSizeBytes, 10)
			cvmiStatus.Size.Unpacked = finalReport.UnpackedSize()
			cvmiStatus.Size.UnpackedBytes = strconv.FormatUint(finalReport.UnpackedSizeBytes, 10)
		}
	}

	state.CVMI.Changed().Status = *cvmiStatus

	return nil
}

func (r *CVMIReconciler) isInited(cvmi *virtv2alpha1.ClusterVirtualMachineImage, state *CVMIReconcilerState) bool {
	switch cvmi.Spec.DataSource.Type {
	case virtv2alpha1.DataSourceTypeUpload:
		return state.HasUploaderAnno()
	default:
		return state.HasImporterAnno()
	}
}

func (r *CVMIReconciler) canStart(cvmi *virtv2alpha1.ClusterVirtualMachineImage, state *CVMIReconcilerState) bool {
	if r.isReady(cvmi, state) || state.Pod != nil {
		return false
	}

	return true
}

func (r *CVMIReconciler) isInProgress(cvmi *virtv2alpha1.ClusterVirtualMachineImage, state *CVMIReconcilerState) bool {
	if state.Pod == nil {
		return false
	}

	return r.isInited(cvmi, state) && state.Pod.Status.Phase == corev1.PodRunning
}

func (r *CVMIReconciler) isInPending(cvmi *virtv2alpha1.ClusterVirtualMachineImage, state *CVMIReconcilerState) bool {
	if state.Pod == nil {
		return false
	}

	return r.isInited(cvmi, state) && state.Pod.Status.Phase == corev1.PodPending
}

func (r *CVMIReconciler) isImportComplete(state *CVMIReconcilerState) bool {
	return state.Pod != nil && cc.IsPodComplete(state.Pod)
}

func (r *CVMIReconciler) isReady(cvmi *virtv2alpha1.ClusterVirtualMachineImage, state *CVMIReconcilerState) bool {
	if state.CVMI.IsEmpty() {
		return false
	}

	if !r.isInited(cvmi, state) {
		return false
	}

	return state.CVMI.Current().Status.Phase == virtv2alpha1.ImageReady
}

func (r *CVMIReconciler) cleanup(ctx context.Context, cvmi *virtv2alpha1.ClusterVirtualMachineImage, client client.Client, state *CVMIReconcilerState) error {
	switch cvmi.Spec.DataSource.Type {
	case virtv2alpha1.DataSourceTypeUpload:
		if err := uploader.CleanupService(ctx, client, state.Service); err != nil {
			return err
		}

		return uploader.CleanupPod(ctx, client, state.Pod)
	default:
		return importer.CleanupPod(ctx, client, state.Pod)
	}
}

func (r *CVMIReconciler) start(ctx context.Context, cvmi *virtv2alpha1.ClusterVirtualMachineImage, opts two_phase_reconciler.ReconcilerOptions) error {
	switch cvmi.Spec.DataSource.Type {
	case virtv2alpha1.DataSourceTypeUpload:
		if err := r.startUploaderPod(ctx, cvmi, opts); err != nil {
			return err
		}

		if err := r.startUploaderService(ctx, cvmi, opts); err != nil {
			return err
		}
	default:
		if err := r.startImporterPod(ctx, cvmi, opts); err != nil {
			return err
		}
	}

	return nil
}

// initCVMIPodName creates new name and update it in the annotation.
// TODO make it work with VMI also
func (r *CVMIReconciler) init(cvmi *virtv2alpha1.ClusterVirtualMachineImage) {
	anno := cvmi.GetAnnotations()
	if anno == nil {
		anno = make(map[string]string)
	}

	switch cvmi.Spec.DataSource.Type {
	case virtv2alpha1.DataSourceTypeUpload:
		anno[cc.AnnUploaderNamespace] = r.namespace
		anno[cc.AnnUploadPodName] = fmt.Sprintf("%s-%s", common.UploaderPodNamePrefix, cvmi.GetName())
		anno[cc.AnnUploadServiceName] = fmt.Sprintf("%s-%s", common.UploaderServiceNamePrefix, cvmi.GetName())
	default:
		anno[cc.AnnImporterNamespace] = r.namespace
		anno[cc.AnnImportPodName] = fmt.Sprintf("%s-%s", common.ImporterPodNamePrefix, cvmi.GetName())
		// Generate name for secret with certs from caBundle.
		if cvmiutil.HasCABundle(cvmi) {
			anno[cc.AnnCABundleConfigMap] = fmt.Sprintf("%s-ca", cvmi.Name)
		}
	}

	cvmi.SetAnnotations(anno)
}
