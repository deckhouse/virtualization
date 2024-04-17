package controller

import (
	"context"

	"github.com/deckhouse/virtualization-controller/pkg/common/datasource"
	vmdutil "github.com/deckhouse/virtualization-controller/pkg/common/vmd"
	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/controller/uploader"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
)

func (r *VMDReconciler) startUploaderPod(ctx context.Context, state *VMDReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	vmd := state.VMD.Current()

	opts.Log.V(1).Info("Creating uploader POD for VD", "vd.Name", vmd.Name)

	uploaderSettings := r.createUploaderSettings(state)

	podSettings := r.createUploaderPodSettings(state)

	uploaderPod := uploader.NewPod(podSettings, uploaderSettings)

	pod, err := uploaderPod.Create(ctx, opts.Client)
	if err != nil {
		err = cc.PublishPodErr(err, podSettings.Name, vmd, opts.Recorder, opts.Client)
		if err != nil {
			return err
		}
	}

	opts.Log.V(1).Info("Created uploader POD", "pod.Name", pod.Name)

	// Ensure supplement resources for the Pod.
	return supplements.EnsureForPod(ctx, opts.Client, state.Supplements, pod, datasource.NewCABundleForVMD(vmd.Spec.DataSource), r.dvcrSettings)
}

// createUploaderSettings fills settings for the dvcr-uploader binary.
func (r *VMDReconciler) createUploaderSettings(state *VMDReconcilerState) *uploader.Settings {
	vmd := state.VMD.Current()
	settings := &uploader.Settings{
		Verbose: r.verbose,
	}

	// Set DVCR destination settings.
	dvcrDestImageName := r.dvcrSettings.RegistryImageForVMD(vmd.Name, vmd.Namespace)
	uploader.ApplyDVCRDestinationSettings(settings, r.dvcrSettings, state.Supplements, dvcrDestImageName)

	// TODO Update proxy settings.

	return settings
}

func (r *VMDReconciler) createUploaderPodSettings(state *VMDReconcilerState) *uploader.PodSettings {
	uploaderPod := state.Supplements.UploaderPod()
	uploaderSvc := state.Supplements.UploaderService()
	return &uploader.PodSettings{
		Name:            uploaderPod.Name,
		Image:           r.uploaderImage,
		PullPolicy:      r.pullPolicy,
		Namespace:       uploaderPod.Namespace,
		OwnerReference:  vmdutil.MakeOwnerReference(state.VMD.Current()),
		ControllerName:  vmdControllerName,
		InstallerLabels: map[string]string{},
		ServiceName:     uploaderSvc.Name,
	}
}

func (r *VMDReconciler) startUploaderService(ctx context.Context, state *VMDReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	opts.Log.V(1).Info("Creating uploader Service for VD", "vd.Name", state.VMD.Current().Name)

	uploaderService := uploader.NewService(r.createUploaderServiceSettings(state))

	service, err := uploaderService.Create(ctx, opts.Client)
	if err != nil {
		return err
	}

	opts.Log.V(1).Info("Created uploader Service", "service.Name", service.Name)

	return nil
}

func (r *VMDReconciler) createUploaderServiceSettings(state *VMDReconcilerState) *uploader.ServiceSettings {
	uploaderSvc := state.Supplements.UploaderService()
	return &uploader.ServiceSettings{
		Name:           uploaderSvc.Name,
		Namespace:      uploaderSvc.Namespace,
		OwnerReference: vmdutil.MakeOwnerReference(state.VMD.Current()),
	}
}

func (r *VMDReconciler) startUploaderIngress(ctx context.Context, state *VMDReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	opts.Log.V(1).Info("Creating uploader Ingress for VD", "vd.Name", state.VMD.Current().Name)

	uploaderIng := uploader.NewIngress(r.createUploaderIngressSettings(state))

	ing, err := uploaderIng.Create(ctx, opts.Client)
	if err != nil {
		return err
	}

	opts.Log.V(1).Info("Created uploader Ingress", "ingress.Name", ing.Name)
	return supplements.EnsureForIngress(ctx, state.Client, state.Supplements, ing, r.dvcrSettings)
}

func (r *VMDReconciler) createUploaderIngressSettings(state *VMDReconcilerState) *uploader.IngressSettings {
	uploaderIng := state.Supplements.UploaderIngress()
	uploaderSvc := state.Supplements.UploaderService()
	secretName := r.dvcrSettings.UploaderIngressSettings.TLSSecret
	if supplements.ShouldCopyUploaderTLSSecret(r.dvcrSettings, state.Supplements) {
		secretName = state.Supplements.UploaderTLSSecretForIngress().Name
	}
	var class *string
	if c := r.dvcrSettings.UploaderIngressSettings.Class; c != "" {
		class = &c
	}
	return &uploader.IngressSettings{
		Name:           uploaderIng.Name,
		Namespace:      uploaderIng.Namespace,
		Host:           r.dvcrSettings.UploaderIngressSettings.Host,
		TLSSecretName:  secretName,
		ServiceName:    uploaderSvc.Name,
		ClassName:      class,
		OwnerReference: vmdutil.MakeOwnerReference(state.VMD.Current()),
	}
}
