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
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
)

type CVMIReconciler struct {
	image           string
	verbose         string
	pullPolicy      string
	installerLabels map[string]string
	namespace       string
	dvcrSettings    *cc.DVCRSettings
}

func (r *CVMIReconciler) SetupController(_ context.Context, _ manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(&source.Kind{Type: &virtv2alpha1.ClusterVirtualMachineImage{}}, &handler.EnqueueRequestForObject{},
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool { return true },
		},
	); err != nil {
		return err
	}

	matchCvmiFunc := func(k, _ string) bool {
		_, isCvmi := ExtractAttachedCVMIName(k)
		return isCvmi
	}

	if err := ctr.Watch(
		&source.Kind{Type: &virtv2alpha1.VirtualMachine{}},
		handler.EnqueueRequestsFromMapFunc(r.mapFromCVMI),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				return HasLabel(e.Object.GetLabels(), matchCvmiFunc)
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return HasLabel(e.Object.GetLabels(), matchCvmiFunc)
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				return HasLabel(e.ObjectOld.GetLabels(), matchCvmiFunc) ||
					HasLabel(e.ObjectNew.GetLabels(), matchCvmiFunc)
			},
		},
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualMachineInstance: %w", err)
	}
	return nil
}

func (r *CVMIReconciler) mapFromCVMI(obj client.Object) (res []reconcile.Request) {
	for k := range obj.GetLabels() {
		name, isCvmi := ExtractAttachedCVMIName(k)
		if isCvmi {
			res = append(res, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: name},
			})
		}
	}
	return
}

// Sync creates and deletes importer Pod depending on CVMI status.
func (r *CVMIReconciler) Sync(ctx context.Context, _ reconcile.Request, state *CVMIReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	opts.Log.Info("Reconcile required for CVMI", "cvmi.name", state.CVMI.Current().Name, "cvmi.phase", state.CVMI.Current().Status.Phase)
	opts.Log.V(2).Info("CVMI Sync", "ShouldRemoveProtectionFinalizer", state.ShouldRemoveProtectionFinalizer())

	// Change the world depending on states of CVMI and Pod.
	switch {
	case state.IsDeletion():
		// Delete existing Pod if CVMI is deleted.
		opts.Log.V(1).Info("CVMI is being deleted, delete importer Pod", "pod.Name", state.Pod.Name)
		if err := importer.CleanupPod(ctx, opts.Client, state.Pod); err != nil {
			return err
		}
		return nil
	case !state.HasImporterPodAnno():
		opts.Log.V(1).Info("New CVMI observed, update annotations with Pod name and namespace")
		// TODO(i.mikh) This algorithm is from CDI: put annotation on fresh CVMI and run Pod on next call to reconcile. Is it ok?
		r.initCVMIPodName(state.CVMI.Changed())
		// Update annotations and status and restart reconcile to create an importer Pod.
		state.SetReconcilerResult(&reconcile.Result{Requeue: true})
		return nil
	case state.IsReady():
		// Note: state.ShouldReconcile was positive, so state.Pod is not nil and should be deleted.
		// Delete importer Pod when CVMI is marked as ready and stop the reconcile process.
		if cc.ShouldDeletePod(state.CVMI.Current()) {
			opts.Log.V(1).Info("Import done, cleanup importer Pod", "pod.Name", state.Pod.Name)
			return importer.CleanupPod(ctx, opts.Client, state.Pod)
		}
	case state.CanStartImporterPod():
		// Create Pod using name and namespace from annotation.
		opts.Log.V(1).Info("Pod for CVMI not found, create new one")
		// Create importer pod, make sure the CVMI owns it.
		if err := r.startImporterPod(ctx, state.CVMI.Current(), opts); err != nil {
			return err
		}
		// Requeue to wait until Pod become Running.
		state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})
		return nil
	case state.HasImporterPod():
		// Import is in progress, force a re-reconcile in 2 seconds to update status.
		opts.Log.V(2).Info("Requeue: CVMI import is in progress", "cvmi.name", state.CVMI.Current().Name)
		state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})
		return nil
	case state.ShouldRemoveProtectionFinalizer():
		state.RemoveProtectionFinalizer()
		state.SetReconcilerResult(&reconcile.Result{Requeue: true})
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

	// Record event if importer Pod has error.
	// TODO set Failed status if Pod restarts are greater than some threshold?
	if state.Pod != nil && len(state.Pod.Status.ContainerStatuses) > 0 {
		if state.Pod.Status.ContainerStatuses[0].LastTerminationState.Terminated != nil &&
			state.Pod.Status.ContainerStatuses[0].LastTerminationState.Terminated.ExitCode > 0 {
			opts.Recorder.Event(state.CVMI.Current(), corev1.EventTypeWarning, "ErrImportFailed", fmt.Sprintf("importer pod phase '%s', message '%s'", state.Pod.Status.Phase, state.Pod.Status.ContainerStatuses[0].LastTerminationState.Terminated.Message))
		}
	}

	cvmiStatus := state.CVMI.Current().Status.DeepCopy()

	// Set target image name the same way as for the importer Pod.
	cvmiStatus.Target.RegistryURL = cc.PrepareDVCREndpointFromCVMI(state.CVMI.Current(), r.dvcrSettings)

	switch {
	case !state.HasImporterPodAnno() || state.CVMI.Current().Status.Phase == "":
		cvmiStatus.Phase = virtv2alpha1.ImagePending
	case state.IsReady():
		break
	case state.IsImportInProgress():
		// Set CVMI status to Provisioning and copy progress metrics from importer Pod.
		opts.Log.V(2).Info("Fetch progress", "cvmi.name", state.CVMI.Current().Name)
		cvmiStatus.Phase = virtv2alpha1.ImageProvisioning

		progress, err := importer.ProgressFromPod(string(state.CVMI.Current().GetUID()), state.Pod)
		if err != nil {
			opts.Recorder.Event(state.CVMI.Current(), corev1.EventTypeWarning, "ErrGetProgressFailed", "Error fetching progress metrics from importer Pod "+err.Error())
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
	case state.IsImportComplete():
		// Set CVMI status to Ready and update image size from final report of the importer Pod.
		opts.Recorder.Event(state.CVMI.Current(), corev1.EventTypeNormal, "ImportSucceeded", "Import Successful")
		opts.Log.V(1).Info("Import completed successfully")
		cvmiStatus.Phase = virtv2alpha1.ImageReady
		// Cleanup.
		cvmiStatus.Progress = ""
		cvmiStatus.DownloadSpeed = virtv2alpha1.ImageStatusSpeed{}
		finalReport, err := importer.GetFinalImporterReport(state.Pod)
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

func (r *CVMIReconciler) startImporterPod(ctx context.Context, cvmi *virtv2alpha1.ClusterVirtualMachineImage, opts two_phase_reconciler.ReconcilerOptions) error {
	opts.Log.V(1).Info("Creating importer POD for PVC", "pvc.Name", cvmi.Name)

	importerSettings, err := r.createImporterSettings(cvmi)
	if err != nil {
		return err
	}

	// all checks passed, let's create the importer pod!
	podSettings := r.createImporterPodSettings(cvmi)

	caBundleSettings := importer.NewCABundleSettings(cvmiutil.GetCABundle(cvmi), cvmi.Annotations[cc.AnnCABundleConfigMap])

	imp := importer.NewImporter(podSettings, importerSettings, caBundleSettings)
	pod, err := imp.CreatePod(ctx, opts.Client)
	// Check if pod has failed and, in that case, record an event with the error
	if podErr := cc.HandleFailedPod(err, cvmi.Annotations[cc.AnnImportPodName], cvmi, opts.Recorder, opts.Client); podErr != nil {
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

func (r *CVMIReconciler) createImporterPodSettings(cvmi *virtv2alpha1.ClusterVirtualMachineImage) *importer.PodSettings {
	return &importer.PodSettings{
		Name:            cvmi.Annotations[cc.AnnImportPodName],
		Image:           r.image,
		PullPolicy:      r.pullPolicy,
		Namespace:       r.namespace, // TODO vmi.Namespace for VirtualMachineImage
		OwnerReference:  cvmiutil.MakeOwnerReference(cvmi),
		ControllerName:  cvmiControllerName,
		InstallerLabels: r.installerLabels,
	}
}

// createImporterSettings fills settings for the registry-importer binary.
func (r *CVMIReconciler) createImporterSettings(cvmi *virtv2alpha1.ClusterVirtualMachineImage) (*importer.Settings, error) {
	settings := &importer.Settings{
		Verbose: r.verbose,
		Source:  cc.GetSource(cvmi.Spec.DataSource),
	}

	switch settings.Source {
	case cc.SourceHTTP:
		if http := cvmi.Spec.DataSource.HTTP; http != nil {
			importer.UpdateHTTPSettings(settings, http)
		}
	case cc.SourceNone:
	default:
		return nil, fmt.Errorf("unknown settings source: %s", settings.Source)
	}

	// Set DVCR settings.
	importer.UpdateDVCRSettings(settings, r.dvcrSettings, cc.PrepareDVCREndpointFromCVMI(cvmi, r.dvcrSettings))

	// TODO Update proxy settings.

	return settings, nil
}

// initCVMIPodName creates new name and update it in the annotation.
// TODO make it work with VMI also
func (r *CVMIReconciler) initCVMIPodName(cvmi *virtv2alpha1.ClusterVirtualMachineImage) {
	anno := cvmi.GetAnnotations()
	if anno == nil {
		anno = make(map[string]string)
	}

	anno[cc.AnnImportPodName] = fmt.Sprintf("%s-%s", common.ImporterPodNamePrefix, cvmi.GetName())
	anno[cc.AnnImportPodNamespace] = r.namespace
	// Generate name for secret with certs from caBundle.
	if cvmiutil.HasCABundle(cvmi) {
		anno[cc.AnnCABundleConfigMap] = fmt.Sprintf("%s-ca", cvmi.Name)
	}

	cvmi.Annotations = anno
}
