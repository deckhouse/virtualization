package controller

import (
	"context"

	"github.com/deckhouse/virtualization-controller/pkg/common/datasource"
	vmiutil "github.com/deckhouse/virtualization-controller/pkg/common/vmi"
	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/controller/uploader"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
)

func (r *VMIReconciler) startUploaderPod(ctx context.Context, state *VMIReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	vmi := state.VMI.Current()

	opts.Log.V(1).Info("Creating uploader POD for VI", "vi.Name", vmi.Name)

	uploaderSettings := r.createUploaderSettings(state)

	podSettings := r.createUploaderPodSettings(state)

	uploaderPod := uploader.NewPod(podSettings, uploaderSettings)

	pod, err := uploaderPod.Create(ctx, opts.Client)
	if err != nil {
		err = cc.PublishPodErr(err, podSettings.Name, vmi, opts.Recorder, opts.Client)
		if err != nil {
			return err
		}
	}

	opts.Log.V(1).Info("Created uploader POD", "pod.Name", pod.Name)

	// Ensure supplement resources for the Pod.
	return supplements.EnsureForPod(ctx, opts.Client, state.Supplements, pod, datasource.NewCABundleForVMI(vmi.Spec.DataSource), r.dvcrSettings)
}

// createUploaderSettings fills settings for the dvcr-uploader binary.
func (r *VMIReconciler) createUploaderSettings(state *VMIReconcilerState) *uploader.Settings {
	vmi := state.VMI.Current()
	settings := &uploader.Settings{
		Verbose: r.verbose,
	}

	// Set DVCR destination settings.
	dvcrDestImageName := r.dvcrSettings.RegistryImageForVMI(vmi.Name, vmi.Namespace)
	uploader.ApplyDVCRDestinationSettings(settings, r.dvcrSettings, state.Supplements, dvcrDestImageName)

	// TODO Update proxy settings.

	return settings
}

func (r *VMIReconciler) createUploaderPodSettings(state *VMIReconcilerState) *uploader.PodSettings {
	uploaderPod := state.Supplements.UploaderPod()
	uploaderSvc := state.Supplements.UploaderService()
	return &uploader.PodSettings{
		Name:            uploaderPod.Name,
		Image:           r.uploaderImage,
		PullPolicy:      r.pullPolicy,
		Namespace:       uploaderPod.Namespace,
		OwnerReference:  vmiutil.MakeOwnerReference(state.VMI.Current()),
		ControllerName:  vmiControllerName,
		InstallerLabels: map[string]string{},
		ServiceName:     uploaderSvc.Name,
	}
}

func (r *VMIReconciler) startUploaderService(ctx context.Context, state *VMIReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	opts.Log.V(1).Info("Creating uploader Service for VI", "vi.Name", state.VMI.Current().Name)

	uploaderService := uploader.NewService(r.createUploaderServiceSettings(state))

	service, err := uploaderService.Create(ctx, opts.Client)
	if err != nil {
		return err
	}

	opts.Log.V(1).Info("Created uploader Service", "service.Name", service.Name)

	return nil
}

func (r *VMIReconciler) createUploaderServiceSettings(state *VMIReconcilerState) *uploader.ServiceSettings {
	uploaderSvc := state.Supplements.UploaderService()
	return &uploader.ServiceSettings{
		Name:           uploaderSvc.Name,
		Namespace:      uploaderSvc.Namespace,
		OwnerReference: vmiutil.MakeOwnerReference(state.VMI.Current()),
	}
}

func (r *VMIReconciler) startUploaderIngress(ctx context.Context, state *VMIReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	opts.Log.V(1).Info("Creating uploader Ingress for VI", "vi.Name", state.VMI.Current().Name)

	uploaderIng := uploader.NewIngress(r.createUploaderIngressSettings(state))

	ing, err := uploaderIng.Create(ctx, opts.Client)
	if err != nil {
		return err
	}

	opts.Log.V(1).Info("Created uploader Ingress", "ingress.Name", ing.Name)
	return supplements.EnsureForIngress(ctx, state.Client, state.Supplements, ing, r.dvcrSettings)
}

func (r *VMIReconciler) createUploaderIngressSettings(state *VMIReconcilerState) *uploader.IngressSettings {
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
		OwnerReference: vmiutil.MakeOwnerReference(state.VMI.Current()),
	}
}
