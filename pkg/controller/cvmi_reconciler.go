package controller

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	virtv2alpha1 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	"github.com/deckhouse/virtualization-controller/pkg/common"
	cvmiutil "github.com/deckhouse/virtualization-controller/pkg/common/cvmi"
	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
)

type CVMIReconciler struct {
	client          client.Client
	recorder        record.EventRecorder
	scheme          *runtime.Scheme
	log             logr.Logger
	image           string
	verbose         string
	pullPolicy      string
	installerLabels map[string]string
	namespace       string
	dvcrSettings    *cc.DVCRSettings
}

// Reconcile loop for CVMIReconciler.
func (r *CVMIReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := r.log.WithValues("CVMI", req.NamespacedName)
	r.log.Info(fmt.Sprintf("Start Reconcile for CVMI: %s", req.String()))

	// Get enqueued ClusterVirtualMachineImage resource.
	cvmi := &virtv2alpha1.ClusterVirtualMachineImage{}
	if err := r.client.Get(ctx, req.NamespacedName, cvmi); err != nil {
		if k8serrors.IsNotFound(err) {
			r.log.Info(fmt.Sprintf("Reconcile observe absent CVMI: %s, it may be deleted", req.String()))
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	// Check if reconcile is required.
	if cc.IsCVMIComplete(cvmi) {
		log.V(2).Info("Should not reconcile this CVMI",
			"cvmi.name", cvmi.Name)
		return reconcile.Result{}, nil
	}

	r.log.Info("Reconcile required for CVMI", "cvmi.name", cvmi.Name)
	return r.reconcileCVMI(cvmi, log)
}

func (r *CVMIReconciler) reconcileCVMI(cvmi *virtv2alpha1.ClusterVirtualMachineImage, log logr.Logger) (reconcile.Result, error) {
	// See if we have a pod associated with the CVMI, we know the CVMI has the needed annotations.
	pod, err := r.findImporterPod(cvmi, log)
	if err != nil {
		return reconcile.Result{}, err
	}

	if pod == nil {
		if cc.IsCVMIComplete(cvmi) {
			// Don't create the Pod if the CVMI is completed already
			log.V(1).Info("CVMI is already complete")
		} else if cvmi.DeletionTimestamp == nil {
			log.V(1).Info("Pod for CVMI not found, create new one")
			if _, ok := cvmi.Annotations[cc.AnnImportPod]; ok {
				// Create importer pod, make sure the CVMI owns it.
				if err := r.startImporterPod(cvmi); err != nil {
					return reconcile.Result{}, err
				}
				// Requeue to update CVMI Status.
				return reconcile.Result{Requeue: true}, nil
			}
			// TODO(i.mikh) This algorithm is from CDI: put annotation on fresh CVMI and run Pod on next call to reconcile. Is it ok?
			if err := r.initCVMIPodName(cvmi, log); err != nil {
				return reconcile.Result{}, err
			}
		}
		// TODO set CVMI status if Pod is exists and CVMI not complete.
	} else {
		if cvmi.DeletionTimestamp != nil {
			log.V(1).Info("CVMI being deleted, delete pod", "pod.Name", pod.Name)
			if err := importer.CleanupPod(context.TODO(), r.client, pod); err != nil {
				return reconcile.Result{}, err
			}
			// TODO research deletion process with finalizers.
		} else {
			// Copy import proxy ConfigMap (if exists) from cdi namespace to the import namespace
			// if err := r.copyImportProxyConfigMap(pvc, pod); err != nil {
			//	return reconcile.Result{}, err
			// }

			// Pod exists, we need to update the CVMI status.
			log.V(1).Info("CVMI import not finished, update progress", "cvmi.name", cvmi.Name)
			if err := r.updateCVMIFromPod(cvmi, pod, log); err != nil {
				return reconcile.Result{}, err
			}
		}
	}

	if !cc.IsCVMIComplete(cvmi) {
		// We are not done yet, force a re-reconcile in 2 seconds to get an update.
		log.V(1).Info("Force Reconcile: CVMI import not finished", "cvmi.name", cvmi.Name)
		return reconcile.Result{RequeueAfter: 2 * time.Second}, nil
	}
	return reconcile.Result{}, nil
}

// findImporterPod returns the Pod using annotation with its name on the CVMI resource.
func (r *CVMIReconciler) findImporterPod(cvmi *virtv2alpha1.ClusterVirtualMachineImage, log logr.Logger) (*corev1.Pod, error) {
	podName := fmt.Sprintf("%s-%s", common.ImporterPodNamePrefix, cvmi.GetName())
	podNS := r.namespace
	pod := &corev1.Pod{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: podName, Namespace: podNS}, pod); err != nil {
		if !k8serrors.IsNotFound(err) {
			return nil, fmt.Errorf("get importer pod %s/%s: %w", podNS, podName, err)
		}
		return nil, nil
	}
	if !metav1.IsControlledBy(pod, cvmi) {
		return nil, fmt.Errorf("get importer pod %s/%s: pod is not owned by CVMI/%s", podNS, podName, cvmi.Name)
	}
	log.V(1).Info("Pod is owned by CVMI", "cvmi.name", cvmi.Name, "pod.name", pod.Name)
	return pod, nil
}

func (r *CVMIReconciler) cleanup(_ *virtv2alpha1.ClusterVirtualMachineImage, pod *corev1.Pod, _ logr.Logger) error {
	if err := r.client.Delete(context.TODO(), pod); cc.IgnoreNotFound(err) != nil {
		return err
	}
	// if cc.HasFinalizer(pvc, importPodImageStreamFinalizer) {
	//	cc.RemoveFinalizer(pvc, importPodImageStreamFinalizer)
	//	if err := r.updatePVC(pvc, log); err != nil {
	//		return err
	//	}
	// }
	return nil
}

// updateCVMIFromPod updates CVMI status from Pod state.
func (r *CVMIReconciler) updateCVMIFromPod(cvmi *virtv2alpha1.ClusterVirtualMachineImage, pod *corev1.Pod, log logr.Logger) error {
	// Make copies for comparison with the original later.
	cvmiCopy := cvmi.DeepCopy()
	cvmiStatus := cvmi.Status.DeepCopy()

	log.V(1).Info("Updating CVMI from pod")
	anno := cvmiCopy.GetAnnotations()

	podRestarts := ""
	if len(pod.Status.ContainerStatuses) > 0 {
		if pod.Status.ContainerStatuses[0].LastTerminationState.Terminated != nil &&
			pod.Status.ContainerStatuses[0].LastTerminationState.Terminated.ExitCode > 0 {
			r.recorder.Event(cvmi, corev1.EventTypeWarning, "ErrImportFailed", pod.Status.ContainerStatuses[0].LastTerminationState.Terminated.Message)
		}
		podRestarts = strconv.Itoa(int(pod.Status.ContainerStatuses[0].RestartCount))
	}

	isComplete := false
	//nolint:gocritic
	if cvmiStatus.Phase == "" {
		cvmiStatus.Phase = string(virtv2alpha1.ImagePending)
		cvmiStatus.Target.RegistryURL = importer.GetDestinationImageNameFromPod(pod)
	} else if cc.IsPodComplete(pod) {
		r.recorder.Event(cvmi, corev1.EventTypeNormal, "ImportSucceeded", "Import Successful")
		log.V(1).Info("Import completed successfully")
		anno[cc.AnnImportDone] = "true"
		isComplete = true
		cvmiStatus.Phase = string(virtv2alpha1.ImageReady)
		// Cleanup progress
		cvmiStatus.Progress = ""
		cvmiStatus.DownloadSpeed.Avg = ""
		cvmiStatus.DownloadSpeed.Current = ""
		finalReport, err := importer.ImporterFinalReport(pod)
		if err != nil {
			log.Error(err, "parsing final report", "cvmi.name", cvmi.Name)
		}
		if finalReport != nil {
			cvmiStatus.Size.Stored = finalReport.StoredSize()
			cvmiStatus.Size.Unpacked = finalReport.UnpackedSize()
			delete(anno, cc.AnnImportAvgSpeedBytes)
			delete(anno, cc.AnnImportCurrentSpeedBytes)
			anno[cc.AnnImportStoredSizeBytes] = strconv.FormatUint(finalReport.StoredSizeBytes, 10)
			anno[cc.AnnImportUnpackedSizeBytes] = strconv.FormatUint(finalReport.UnpackedSizeBytes, 10)
		}
	} else if pod.Status.Phase == corev1.PodRunning {
		// Copy progress from Pod metrics to cvmi.Status.
		// TODO Is UID important? Why not just get metric values without checking UID label?
		log.V(2).Info("Fetch progress", "cvmi.name", cvmi.Name)
		cvmiStatus.Phase = string(virtv2alpha1.ImageProvisioning)

		progress, err := importer.ProgressFromPod(string(cvmi.GetUID()), pod)
		if err != nil {
			r.recorder.Event(cvmi, corev1.EventTypeWarning, "ErrGetProgressFailed", "Error fetching progress metrics from importer Pod "+err.Error())
			return err
		}
		if progress != nil {
			log.V(2).Info("Got progress", "cvmi.name", cvmi.Name, "progress", progress.Progress(), "speed", progress.AvgSpeed(), "progress.raw", progress.ProgressRaw(), "speed.raw", progress.AvgSpeedRaw())
			cvmiStatus.Progress = progress.Progress()
			cvmiStatus.DownloadSpeed.Avg = progress.AvgSpeed()
			cvmiStatus.DownloadSpeed.Current = progress.CurrentSpeed()
			anno[cc.AnnImportAvgSpeedBytes] = strconv.FormatUint(progress.AvgSpeedRaw(), 10)
			anno[cc.AnnImportCurrentSpeedBytes] = strconv.FormatUint(progress.CurrentSpeedRaw(), 10)
		}

		log.V(2).Info("Status.Progress", "cvmi", cvmi.Status.Progress, "copy", cvmiStatus.Progress)
	}

	anno[cc.AnnImportPod] = pod.Name
	anno[cc.AnnCurrentPodID] = string(pod.ObjectMeta.UID)
	anno[cc.AnnPodPhase] = string(pod.Status.Phase)
	anno[cc.AnnPodRestarts] = podRestarts

	// Update annotations.
	if !reflect.DeepEqual(cvmiCopy, cvmi) {
		if err := r.updateCVMI(cvmiCopy, log); err != nil {
			return fmt.Errorf("update cvmi: %w", err)
		}
		log.V(1).Info("Updated CVMI", "cvmi.anno.Phase", anno[cc.AnnPodPhase], "cvmi.anno.Restarts", anno[cc.AnnPodRestarts])
	}

	if reflect.DeepEqual(cvmiCopy, cvmi) {
		log.V(1).Info("Updated CVMI: copy is equal to original!")
	}

	// Update status.
	if !reflect.DeepEqual(cvmiStatus, cvmi.Status) {
		cvmiCopy.Status = *cvmiStatus
		if err := r.client.Status().Update(context.TODO(), cvmiCopy); err != nil {
			return fmt.Errorf("update status: %w", err)
		}
		log.V(1).Info("Updated CVMI Status", "cvmi.status.Phase", cvmiCopy.Status.Phase)
	}

	// Cleanup if succeeded.
	if isComplete && cc.IsPodComplete(pod) && cc.ShouldDeletePod(cvmi) {
		log.V(1).Info("Deleting pod", "pod.Name", pod.Name)
		if err := r.cleanup(cvmi, pod, log); err != nil {
			return err
		}
	}

	return nil
}

func (r *CVMIReconciler) startImporterPod(cvmi *virtv2alpha1.ClusterVirtualMachineImage) error {
	r.log.V(1).Info("Creating importer POD for PVC", "pvc.Name", cvmi.Name)
	var err error

	importerSettings := r.createImporterSettings(cvmi)
	// all checks passed, let's create the importer pod!
	podSettings := r.createImporterPodSettings(cvmi)

	caBundleSettings := importer.NewCABundleSettings(cvmiutil.GetCABundle(cvmi), cvmi.Annotations[cc.AnnCABundleConfigMap])

	imp := importer.NewImporter(podSettings, importerSettings, caBundleSettings)
	pod, err := imp.CreatePod(context.TODO(), r.client)
	// Check if pod has failed and, in that case, record an event with the error
	if podErr := cc.HandleFailedPod(err, cvmi.Annotations[cc.AnnImportPod], cvmi, r.recorder, r.client); podErr != nil {
		return podErr
	}

	r.log.V(1).Info("Created importer POD", "pod.Name", pod.Name)

	if caBundleSettings != nil {
		if err := imp.EnsureCABundleConfigMap(context.TODO(), r.client, pod); err != nil {
			return fmt.Errorf("create ConfigMap with certs from caBundle: %w", err)
		}
		r.log.V(1).Info("Created ConfigMap with caBundle", "cm.Name", caBundleSettings.ConfigMapName)
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
		Name:            cvmi.Annotations[cc.AnnImportPod],
		Image:           r.image,
		PullPolicy:      r.pullPolicy,
		Namespace:       r.namespace, // TODO vmi.Namespace for VirtualMachineImage
		OwnerReference:  cvmiutil.MakeOwnerReference(cvmi),
		ControllerName:  cvmiControllerName,
		InstallerLabels: r.installerLabels,
	}
}

// createImporterSettings fills settings for the registry-importer binary.
func (r *CVMIReconciler) createImporterSettings(cvmi *virtv2alpha1.ClusterVirtualMachineImage) *importer.Settings {
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
	}

	// Set DVCR settings.
	importer.UpdateDVCRSettings(settings, r.dvcrSettings, cc.PrepareDVCREndpointFromCVMI(cvmi, r.dvcrSettings))

	// TODO Update proxy settings.

	return settings
}

// initCVMIPodName creates new name and update it in the annotation.
// TODO make it work with VMI also
func (r *CVMIReconciler) initCVMIPodName(cvmi *virtv2alpha1.ClusterVirtualMachineImage, log logr.Logger) error {
	cvmiCopy := cvmi.DeepCopyObject()

	log.V(1).Info("Init pod name on CVMI")
	anno := cvmi.GetAnnotations()
	anno[cc.AnnImportPod] = fmt.Sprintf("%s-%s", common.ImporterPodNamePrefix, cvmi.GetName())
	// Generate name for secret with certs from caBundle.
	if cvmiutil.HasCABundle(cvmi) {
		anno[cc.AnnCABundleConfigMap] = fmt.Sprintf("%s-ca", cvmi.Name)
	}

	// TODO return state

	if !reflect.DeepEqual(cvmiCopy, cvmi) {
		if err := r.updateCVMI(cvmi, log); err != nil {
			return err
		}
		log.V(1).Info("Updated CVMI", "cvmi.anno.AnnImportPod", anno[cc.AnnImportPod])
	}
	return nil
}

// TODO migrate to framework
func (r *CVMIReconciler) updateCVMI(cvmi *virtv2alpha1.ClusterVirtualMachineImage, log logr.Logger) error {
	log.V(1).Info("Annotations are now", "cvmi.anno", cvmi.GetAnnotations())
	return r.client.Update(context.TODO(), cvmi)
}
