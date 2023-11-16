package controller

import (
	"context"

	virtv2alpha1 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	vmdutil "github.com/deckhouse/virtualization-controller/pkg/common/vmd"
	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/uploader"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
)

func (r *VMDReconciler) startUploaderPod(ctx context.Context, vmd *virtv2alpha1.VirtualMachineDisk, opts two_phase_reconciler.ReconcilerOptions) error {
	opts.Log.V(1).Info("Creating uploader POD for PVC", "pvc.Name", vmd.Name)

	uploaderSettings := r.createUploaderSettings(vmd)

	podSettings := r.createUploaderPodSettings(vmd)

	uploaderPod := uploader.NewPod(podSettings, uploaderSettings)

	pod, err := uploaderPod.Create(ctx, opts.Client)
	if err != nil {
		err = cc.PublishPodErr(err, vmd.Annotations[cc.AnnUploadPodName], vmd, opts.Recorder, opts.Client)
		if err != nil {
			return err
		}
	}

	opts.Log.V(1).Info("Created uploader POD", "pod.Name", pod.Name)

	return nil
}

// createUploaderSettings fills settings for the dvcr-uploader binary.
func (r *VMDReconciler) createUploaderSettings(vmd *virtv2alpha1.VirtualMachineDisk) *uploader.Settings {
	settings := &uploader.Settings{
		Verbose: r.verbose,
	}

	// Set DVCR settings.
	uploader.UpdateDVCRSettings(settings, r.dvcrSettings, dvcr.RegistryImageName(r.dvcrSettings, dvcr.ImagePathForVMD(vmd)))

	// TODO Update proxy settings.

	return settings
}

func (r *VMDReconciler) createUploaderPodSettings(vmd *virtv2alpha1.VirtualMachineDisk) *uploader.PodSettings {
	return &uploader.PodSettings{
		Name:            vmd.Annotations[cc.AnnUploadPodName],
		Image:           r.uploaderImage,
		PullPolicy:      r.pullPolicy,
		Namespace:       vmd.GetNamespace(),
		OwnerReference:  vmdutil.MakeOwnerReference(vmd),
		ControllerName:  vmdControllerName,
		InstallerLabels: map[string]string{},
		ServiceName:     vmd.Annotations[cc.AnnUploadServiceName],
	}
}

func (r *VMDReconciler) startUploaderService(ctx context.Context, vmd *virtv2alpha1.VirtualMachineDisk, opts two_phase_reconciler.ReconcilerOptions) error {
	opts.Log.V(1).Info("Creating uploader Service for PVC", "pvc.Name", vmd.Name)

	uploaderService := uploader.NewService(r.createUploaderServiceSettings(vmd))

	service, err := uploaderService.Create(ctx, opts.Client)
	if err != nil {
		return err
	}

	opts.Log.V(1).Info("Created uploader Service", "service.Name", service.Name)

	return nil
}

func (r *VMDReconciler) createUploaderServiceSettings(vmd *virtv2alpha1.VirtualMachineDisk) *uploader.ServiceSettings {
	return &uploader.ServiceSettings{
		Name:           vmd.Annotations[cc.AnnUploadServiceName],
		Namespace:      vmd.GetNamespace(),
		OwnerReference: vmdutil.MakeOwnerReference(vmd),
	}
}
