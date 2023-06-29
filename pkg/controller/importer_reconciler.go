package controller

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
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
	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
)

type ImporterReconciler struct {
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
func (r *ImporterReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
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
		log.V(3).Info("Should not reconcile this CVMI",
			"cvmi.name", cvmi.Name)
		return reconcile.Result{}, nil
	}

	r.log.Info("Reconcile required for CVMI", "cvmi.name", cvmi.Name)
	return r.reconcileCVMI(cvmi, log)
}

func (r *ImporterReconciler) reconcileCVMI(cvmi *virtv2alpha1.ClusterVirtualMachineImage, log logr.Logger) (reconcile.Result, error) {
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
				if err := r.createImporterPod(cvmi); err != nil {
					// TODO update status
					return reconcile.Result{}, err
				}
			} else {
				// TODO(i.mikh) This algorithm is from CDI: put annotation on fresh CVMI and run Pod on next call to reconcile. Is it ok?
				if err := r.initCVMIPodName(cvmi, log); err != nil {
					return reconcile.Result{}, err
				}
			}
		}
	} else {
		if cvmi.DeletionTimestamp != nil {
			// TODO research deletion process with finalizers.
			log.V(1).Info("CVMI being deleted, delete pods", "pod.Name", pod.Name)
			if err := r.cleanup(cvmi, pod, log); err != nil {
				return reconcile.Result{}, err
			}
		} else {
			// Copy import proxy ConfigMap (if exists) from cdi namespace to the import namespace
			//if err := r.copyImportProxyConfigMap(pvc, pod); err != nil {
			//	return reconcile.Result{}, err
			//}

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
func (r *ImporterReconciler) findImporterPod(cvmi *virtv2alpha1.ClusterVirtualMachineImage, log logr.Logger) (*corev1.Pod, error) {
	podName := fmt.Sprintf("%s-%s", common.ImporterPodName, cvmi.GetName())
	podNS := r.namespace
	pod := &corev1.Pod{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: podName, Namespace: podNS}, pod); err != nil {
		if !k8serrors.IsNotFound(err) {
			return nil, errors.Wrapf(err, "error getting import pod %s/%s", podNS, podName)
		}
		return nil, nil
	}
	if !metav1.IsControlledBy(pod, cvmi) {
		return nil, errors.Errorf("Pod is not owned by CVMI")
	}
	log.V(1).Info("Pod is owned by CVMI", "cvmi.name", cvmi.Name, "pod.name", pod.Name)
	return pod, nil
}

func (r *ImporterReconciler) cleanup(cvmi *virtv2alpha1.ClusterVirtualMachineImage, pod *corev1.Pod, log logr.Logger) error {
	if err := r.client.Delete(context.TODO(), pod); cc.IgnoreNotFound(err) != nil {
		return err
	}
	//if cc.HasFinalizer(pvc, importPodImageStreamFinalizer) {
	//	cc.RemoveFinalizer(pvc, importPodImageStreamFinalizer)
	//	if err := r.updatePVC(pvc, log); err != nil {
	//		return err
	//	}
	//}
	return nil

}

func (r *ImporterReconciler) updateCVMIFromPod(cvmi *virtv2alpha1.ClusterVirtualMachineImage, pod *corev1.Pod, log logr.Logger) error {
	// Keep a copy of the original for comparison later.
	cvmiCopy := cvmi.DeepCopyObject()

	log.V(1).Info("Updating CVMI from pod")
	anno := cvmi.GetAnnotations()
	//setAnnotationsFromPodWithPrefix(anno, pod, cc.AnnRunningCondition)

	if pod.Status.ContainerStatuses != nil &&
		pod.Status.ContainerStatuses[0].LastTerminationState.Terminated != nil &&
		pod.Status.ContainerStatuses[0].LastTerminationState.Terminated.ExitCode > 0 {
		r.recorder.Event(cvmi, corev1.EventTypeWarning, "ErrImportFailed", pod.Status.ContainerStatuses[0].LastTerminationState.Terminated.Message)
	}

	isComplete := false
	if cc.IsPodComplete(pod) {
		r.recorder.Event(cvmi, corev1.EventTypeNormal, "ImportSucceeded", "Import Successful")
		log.V(1).Info("Import completed successfully")
		anno[cc.AnnImportDone] = "true"
		isComplete = true
	}

	anno[cc.AnnCurrentPodID] = string(pod.ObjectMeta.UID)
	anno[cc.AnnImportPod] = string(pod.Name)
	anno[cc.AnnPodPhase] = string(pod.Status.Phase)

	if !reflect.DeepEqual(cvmiCopy, cvmi) {
		if err := r.updateCVMI(cvmi, log); err != nil {
			return err
		}
		log.V(1).Info("Updated CVMI", "cvmi.anno.Phase", anno[cc.AnnPodPhase], "cvmi.anno.Restarts", anno[cc.AnnPodRestarts])
	}

	// Cleanup if succeeded.
	if isComplete && cc.ShouldDeletePod(cvmi) {
		log.V(1).Info("Deleting pod", "pod.Name", pod.Name)
		if err := r.cleanup(cvmi, pod, log); err != nil {
			return err
		}
	}
	return nil
}

func (r *ImporterReconciler) createImporterPod(cvmi *virtv2alpha1.ClusterVirtualMachineImage) error {
	r.log.V(1).Info("Creating importer POD for PVC", "pvc.Name", cvmi.Name)
	var err error

	podEnvVar, err := r.createImportEnvVar(cvmi)
	if err != nil {
		return err
	}
	// all checks passed, let's create the importer pod!
	podArgs := &importerPodArgs{
		image:      r.image,
		verbose:    r.verbose,
		pullPolicy: r.pullPolicy,
		podEnvVar:  podEnvVar,
		namespace:  r.namespace, // TODO vmi.Namespace for VirtualMachineImage
		cvmi:       cvmi,
	}

	pod, err := createImporterPod(context.TODO(), r.log, r.client, podArgs, r.installerLabels)
	// Check if pod has failed and, in that case, record an event with the error
	if podErr := cc.HandleFailedPod(err, cvmi.Annotations[cc.AnnImportPod], cvmi, r.recorder, r.client); podErr != nil {
		return podErr
	}

	r.log.V(1).Info("Created POD", "pod.Name", pod.Name)

	// TODO add finalizer.
	//// If importing from image stream, add finalizer. Note we don't watch the importer pod in this case,
	//// so to prevent a deadlock we add finalizer only if the pod is not retained after completion.
	//if cc.IsImageStream(pvc) && pvc.GetAnnotations()[cc.AnnPodRetainAfterCompletion] != "true" {
	//	cc.AddFinalizer(pvc, importPodImageStreamFinalizer)
	//	if err := r.updatePVC(pvc, r.log); err != nil {
	//		return err
	//	}
	//}

	return nil
}

func (r *ImporterReconciler) createImportEnvVar(cvmi *virtv2alpha1.ClusterVirtualMachineImage) (*cc.ImportPodEnvVar, error) {
	podEnvVar := &cc.ImportPodEnvVar{}
	podEnvVar.Source = cc.GetSource(cvmi)

	switch podEnvVar.Source {
	case cc.SourceHTTP:
		if http := cvmi.Spec.DataSource.HTTP; http != nil {
			cc.UpdateHTTPEnvs(podEnvVar, http)
		}

	}

	// Set DVCR settings.
	cc.UpdateDVCREnvsFromCVMI(podEnvVar, cvmi, r.dvcrSettings)

	// TODO Update proxy envs.

	return podEnvVar, nil
	//
	//var err error
	//if podEnvVar.source != cc.SourceNone {
	//	podEnvVar.ep, err = cc.GetEndpoint(cvmi)
	//	if err != nil {
	//		return nil, err
	//	}
	//	//podEnvVar.secretName = r.getSecretName(pvc)
	//	//if podEnvVar.secretName == "" {
	//	//	r.log.V(2).Info("no secret will be supplied to endpoint", "endPoint", podEnvVar.ep)
	//	//}
	//	//get the CDIConfig to extract the proxy configuration to be used to import an image
	//	//cdiConfig := &cdiv1.CDIConfig{}
	//	//err = r.client.Get(context.TODO(), types.NamespacedName{Name: common.ConfigName}, cdiConfig)
	//	//if err != nil {
	//	//	return nil, err
	//	//}
	//	//podEnvVar.certConfigMap, err = r.getCertConfigMap(pvc)
	//	//if err != nil {
	//	//	return nil, err
	//	//}
	//	//podEnvVar.insecureTLS, err = r.isInsecureTLS(pvc, cdiConfig)
	//	//if err != nil {
	//	//	return nil, err
	//	//}
	//	//podEnvVar.diskID = getValueFromAnnotation(pvc, cc.AnnDiskID)
	//	//podEnvVar.backingFile = getValueFromAnnotation(pvc, cc.AnnBackingFile)
	//	podEnvVar.uuid = getValueFromAnnotation(cvmi, cc.AnnUUID)
	//	podEnvVar.thumbprint = getValueFromAnnotation(cvmi, cc.AnnThumbprint)
	//	podEnvVar.previousCheckpoint = getValueFromAnnotation(cvmi, cc.AnnPreviousCheckpoint)
	//	podEnvVar.currentCheckpoint = getValueFromAnnotation(cvmi, cc.AnnCurrentCheckpoint)
	//	podEnvVar.finalCheckpoint = getValueFromAnnotation(cvmi, cc.AnnFinalCheckpoint)
	//
	//	for annotation, value := range cvmi.Annotations {
	//		if strings.HasPrefix(annotation, cc.AnnExtraHeaders) {
	//			podEnvVar.extraHeaders = append(podEnvVar.extraHeaders, value)
	//		}
	//		if strings.HasPrefix(annotation, cc.AnnSecretExtraHeaders) {
	//			podEnvVar.secretExtraHeaders = append(podEnvVar.secretExtraHeaders, value)
	//		}
	//	}
	//
	//	// TODO support proxy
	//	//var field string
	//	//if field, err = GetImportProxyConfig(cdiConfig, common.ImportProxyHTTP); err != nil {
	//	//	r.log.V(3).Info("no proxy http url will be supplied:", err.Error())
	//	//}
	//	//podEnvVar.httpProxy = field
	//	//if field, err = GetImportProxyConfig(cdiConfig, common.ImportProxyHTTPS); err != nil {
	//	//	r.log.V(3).Info("no proxy https url will be supplied:", err.Error())
	//	//}
	//	//podEnvVar.httpsProxy = field
	//	//if field, err = GetImportProxyConfig(cdiConfig, common.ImportProxyNoProxy); err != nil {
	//	//	r.log.V(3).Info("the noProxy field will not be supplied:", err.Error())
	//	//}
	//	//podEnvVar.noProxy = field
	//	//if field, err = GetImportProxyConfig(cdiConfig, common.ImportProxyConfigMapName); err != nil {
	//	//	r.log.V(3).Info("no proxy CA certiticate will be supplied:", err.Error())
	//	//}
	//	//podEnvVar.certConfigMapProxy = field
	//}
	//
	////fsOverhead, err := GetFilesystemOverhead(context.TODO(), r.client, pvc)
	////if err != nil {
	////	return nil, err
	////}
	////podEnvVar.filesystemOverhead = string(fsOverhead)
	//
	////if preallocation, err := strconv.ParseBool(getValueFromAnnotation(cvmi, cc.AnnPreallocationRequested)); err == nil {
	////	podEnvVar.preallocation = preallocation
	////} // else use the default "false"
	//
	//return podEnvVar, nil
}

// initCVMIPodName creates new name and update it in the annotation.
// TODO make it work with VMI also
func (r *ImporterReconciler) initCVMIPodName(cvmi *virtv2alpha1.ClusterVirtualMachineImage, log logr.Logger) error {
	cvmiCopy := cvmi.DeepCopyObject()

	log.V(1).Info("Init pod name on CVMI")
	anno := cvmi.GetAnnotations()
	anno[cc.AnnImportPod] = fmt.Sprintf("%s-%s", common.ImporterPodName, cvmi.GetName())

	//requiresScratch := r.requiresScratchSpace(pvc)
	//if requiresScratch {
	//	anno[cc.AnnRequiresScratch] = "true"
	//}

	if !reflect.DeepEqual(cvmiCopy, cvmi) {
		if err := r.updateCVMI(cvmi, log); err != nil {
			return err
		}
		log.V(1).Info("Updated CVMI", "cvmi.anno.AnnImportPod", anno[cc.AnnImportPod])
	}
	return nil
}

// TODO make it work with VMI also
func (r *ImporterReconciler) updateCVMI(cvmi *virtv2alpha1.ClusterVirtualMachineImage, log logr.Logger) error {
	log.V(1).Info("Annotations are now", "cvmi.anno", cvmi.GetAnnotations())
	if err := r.client.Update(context.TODO(), cvmi); err != nil {
		return err
	}
	return nil
}
