package controller

import (
	"context"

	virtv2alpha1 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	cvmiutil "github.com/deckhouse/virtualization-controller/pkg/common/cvmi"
	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/uploader"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
)

func (r *CVMIReconciler) startUploaderPod(ctx context.Context, cvmi *virtv2alpha1.ClusterVirtualMachineImage, opts two_phase_reconciler.ReconcilerOptions) error {
	opts.Log.V(1).Info("Creating uploader POD for PVC", "pvc.Name", cvmi.Name)

	uploaderSettings := r.createUploaderSettings(cvmi)

	podSettings := r.createUploaderPodSettings(cvmi)

	uploaderPod := uploader.NewPod(podSettings, uploaderSettings)

	pod, err := uploaderPod.Create(ctx, opts.Client)
	if err != nil {
		err = cc.PublishPodErr(err, cvmi.Annotations[cc.AnnUploadPodName], cvmi, opts.Recorder, opts.Client)
		if err != nil {
			return err
		}
	}

	opts.Log.V(1).Info("Created uploader POD", "pod.Name", pod.Name)

	return nil
}

// createUploaderSettings fills settings for the registry-uploader binary.
func (r *CVMIReconciler) createUploaderSettings(cvmi *virtv2alpha1.ClusterVirtualMachineImage) *uploader.Settings {
	settings := &uploader.Settings{
		Verbose: r.verbose,
	}

	// Set DVCR settings.
	uploader.UpdateDVCRSettings(settings, r.dvcrSettings, cc.PrepareDVCREndpointFromCVMI(cvmi, r.dvcrSettings))

	// TODO Update proxy settings.

	return settings
}

func (r *CVMIReconciler) createUploaderPodSettings(cvmi *virtv2alpha1.ClusterVirtualMachineImage) *uploader.PodSettings {
	return &uploader.PodSettings{
		Name:            cvmi.Annotations[cc.AnnUploadPodName],
		Image:           r.uploaderImage,
		PullPolicy:      r.pullPolicy,
		Namespace:       r.namespace,
		OwnerReference:  cvmiutil.MakeOwnerReference(cvmi),
		ControllerName:  cvmiControllerName,
		InstallerLabels: r.installerLabels,
		ServiceName:     cvmi.Annotations[cc.AnnUploadServiceName],
	}
}

func (r *CVMIReconciler) startUploaderService(ctx context.Context, cvmi *virtv2alpha1.ClusterVirtualMachineImage, opts two_phase_reconciler.ReconcilerOptions) error {
	opts.Log.V(1).Info("Creating uploader Service for PVC", "pvc.Name", cvmi.Name)

	uploaderService := uploader.NewService(r.createUploaderServiceSettings(cvmi))

	service, err := uploaderService.Create(ctx, opts.Client)
	if err != nil {
		return err
	}

	opts.Log.V(1).Info("Created uploader Service", "service.Name", service.Name)

	return nil
}

func (r *CVMIReconciler) createUploaderServiceSettings(cvmi *virtv2alpha1.ClusterVirtualMachineImage) *uploader.ServiceSettings {
	return &uploader.ServiceSettings{
		Name:           cvmi.Annotations[cc.AnnUploadServiceName],
		Namespace:      r.namespace,
		OwnerReference: cvmiutil.MakeOwnerReference(cvmi),
	}
}
