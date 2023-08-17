package controller

import (
	"context"

	virtv2alpha1 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	vmiutil "github.com/deckhouse/virtualization-controller/pkg/common/vmi"
	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/uploader"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
)

func (r *VMIReconciler) startUploaderPod(ctx context.Context, vmi *virtv2alpha1.VirtualMachineImage, opts two_phase_reconciler.ReconcilerOptions) error {
	opts.Log.V(1).Info("Creating uploader POD for PVC", "pvc.Name", vmi.Name)

	uploaderSettings := r.createUploaderSettings(vmi)

	podSettings := r.createUploaderPodSettings(vmi)

	uploaderPod := uploader.NewPod(podSettings, uploaderSettings)

	pod, err := uploaderPod.Create(ctx, opts.Client)
	if err != nil {
		err = cc.PublishPodErr(err, vmi.Annotations[cc.AnnUploadPodName], vmi, opts.Recorder, opts.Client)
		if err != nil {
			return err
		}
	}

	opts.Log.V(1).Info("Created uploader POD", "pod.Name", pod.Name)

	return nil
}

// createUploaderSettings fills settings for the registry-uploader binary.
func (r *VMIReconciler) createUploaderSettings(vmi *virtv2alpha1.VirtualMachineImage) *uploader.Settings {
	settings := &uploader.Settings{
		Verbose: r.verbose,
	}

	// Set DVCR settings.
	uploader.UpdateDVCRSettings(settings, r.dvcrSettings, cc.PrepareDVCREndpointFromVMI(vmi, r.dvcrSettings))

	// TODO Update proxy settings.

	return settings
}

func (r *VMIReconciler) createUploaderPodSettings(vmi *virtv2alpha1.VirtualMachineImage) *uploader.PodSettings {
	return &uploader.PodSettings{
		Name:            vmi.Annotations[cc.AnnUploadPodName],
		Image:           r.uploaderImage,
		PullPolicy:      r.pullPolicy,
		Namespace:       vmi.GetNamespace(),
		OwnerReference:  vmiutil.MakeOwnerReference(vmi),
		ControllerName:  vmiControllerName,
		InstallerLabels: map[string]string{},
		ServiceName:     vmi.Annotations[cc.AnnUploadServiceName],
	}
}

func (r *VMIReconciler) startUploaderService(ctx context.Context, vmi *virtv2alpha1.VirtualMachineImage, opts two_phase_reconciler.ReconcilerOptions) error {
	opts.Log.V(1).Info("Creating uploader Service for PVC", "pvc.Name", vmi.Name)

	uploaderService := uploader.NewService(r.createUploaderServiceSettings(vmi))

	service, err := uploaderService.Create(ctx, opts.Client)
	if err != nil {
		return err
	}

	opts.Log.V(1).Info("Created uploader Service", "service.Name", service.Name)

	return nil
}

func (r *VMIReconciler) createUploaderServiceSettings(vmi *virtv2alpha1.VirtualMachineImage) *uploader.ServiceSettings {
	return &uploader.ServiceSettings{
		Name:           vmi.Annotations[cc.AnnUploadServiceName],
		Namespace:      vmi.GetNamespace(),
		OwnerReference: vmiutil.MakeOwnerReference(vmi),
	}
}
