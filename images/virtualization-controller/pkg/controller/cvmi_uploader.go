package controller

import (
	"context"

	cvmiutil "github.com/deckhouse/virtualization-controller/pkg/common/cvmi"
	"github.com/deckhouse/virtualization-controller/pkg/common/datasource"
	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/controller/uploader"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
)

func (r *CVMIReconciler) startUploaderPod(ctx context.Context, state *CVMIReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	cvmi := state.CVMI.Current()

	opts.Log.V(1).Info("Creating uploader POD for CVMI", "cvmi.Name", cvmi.Name)

	uploaderSettings := r.createUploaderSettings(state)

	podSettings := r.createUploaderPodSettings(state)

	uploaderPod := uploader.NewPod(podSettings, uploaderSettings)

	pod, err := uploaderPod.Create(ctx, opts.Client)
	if err != nil {
		err = cc.PublishPodErr(err, podSettings.Name, cvmi, opts.Recorder, opts.Client)
		if err != nil {
			return err
		}
	}

	opts.Log.V(1).Info("Created uploader POD", "pod.Name", pod.Name)

	// Ensure supplement resources for the Pod.
	return supplements.EnsureForPod(ctx, opts.Client, state.Supplements, pod, datasource.NewCABundleForCVMI(cvmi.Spec.DataSource), r.dvcrSettings)
}

// createUploaderSettings fills settings for the dvcr-uploader binary.
func (r *CVMIReconciler) createUploaderSettings(state *CVMIReconcilerState) *uploader.Settings {
	settings := &uploader.Settings{
		Verbose: r.verbose,
	}

	// Set DVCR destination settings.
	dvcrDestImageName := r.dvcrSettings.RegistryImageForCVMI(state.CVMI.Current().Name)
	uploader.ApplyDVCRDestinationSettings(settings, r.dvcrSettings, state.Supplements, dvcrDestImageName)

	// TODO Update proxy settings.

	return settings
}

func (r *CVMIReconciler) createUploaderPodSettings(state *CVMIReconcilerState) *uploader.PodSettings {
	uploaderPod := state.Supplements.UploaderPod()
	uploaderSvc := state.Supplements.UploaderService()
	return &uploader.PodSettings{
		Name:            uploaderPod.Name,
		Image:           r.uploaderImage,
		PullPolicy:      r.pullPolicy,
		Namespace:       uploaderPod.Namespace,
		OwnerReference:  cvmiutil.MakeOwnerReference(state.CVMI.Current()),
		ControllerName:  cvmiControllerName,
		InstallerLabels: r.installerLabels,
		ServiceName:     uploaderSvc.Name,
	}
}

func (r *CVMIReconciler) startUploaderService(ctx context.Context, state *CVMIReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	opts.Log.V(1).Info("Creating uploader Service for CVMI", "cvmi.Name", state.CVMI.Current().Name)

	uploaderService := uploader.NewService(r.createUploaderServiceSettings(state))

	service, err := uploaderService.Create(ctx, opts.Client)
	if err != nil {
		return err
	}

	opts.Log.V(1).Info("Created uploader Service", "service.Name", service.Name)

	return nil
}

func (r *CVMIReconciler) createUploaderServiceSettings(state *CVMIReconcilerState) *uploader.ServiceSettings {
	uploaderSvc := state.Supplements.UploaderService()
	return &uploader.ServiceSettings{
		Name:           uploaderSvc.Name,
		Namespace:      uploaderSvc.Namespace,
		OwnerReference: cvmiutil.MakeOwnerReference(state.CVMI.Current()),
	}
}

func (r *CVMIReconciler) startUploaderIngress(ctx context.Context, state *CVMIReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	opts.Log.V(1).Info("Creating uploader Ingress for CVMI", "cvmi.Name", state.CVMI.Current().Name)

	uploaderIng := uploader.NewIngress(r.createUploaderIngressSettings(state))

	ing, err := uploaderIng.Create(ctx, opts.Client)
	if err != nil {
		return err
	}

	opts.Log.V(1).Info("Created uploader Ingress", "ingress.Name", ing.Name)

	return supplements.EnsureForIngress(ctx, state.Client, state.Supplements, ing, r.dvcrSettings)
}

func (r *CVMIReconciler) createUploaderIngressSettings(state *CVMIReconcilerState) *uploader.IngressSettings {
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
		OwnerReference: cvmiutil.MakeOwnerReference(state.CVMI.Current()),
	}
}
