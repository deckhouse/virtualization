package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"

	virtv2alpha1 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	cvmiutil "github.com/deckhouse/virtualization-controller/pkg/common/cvmi"
	podutil "github.com/deckhouse/virtualization-controller/pkg/common/pod"
	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
)

func (r *CVMIReconciler) startImporterPod(
	ctx context.Context,
	cvmi *virtv2alpha1.ClusterVirtualMachineImage,
	imgPullSecret *ImagePullSecret,
	opts two_phase_reconciler.ReconcilerOptions,
) error {
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
	if err != nil {
		err = cc.PublishPodErr(err, cvmi.Annotations[cc.AnnImportPodName], cvmi, opts.Recorder, opts.Client)
		if err != nil {
			return err
		}
	}

	opts.Log.V(1).Info("Created importer POD", "pod.Name", pod.Name)

	if caBundleSettings != nil {
		if err := imp.EnsureCABundleConfigMap(ctx, opts.Client, pod); err != nil {
			return fmt.Errorf("create ConfigMap with certs from caBundle: %w", err)
		}
		opts.Log.V(1).Info("Created ConfigMap with caBundle", "cm.Name", caBundleSettings.ConfigMapName)
	}
	if imgPullSecret != nil && imgPullSecret.Secret == nil && imgPullSecret.SourceSecret != nil {
		if err := r.createImporterAuthSecret(ctx, cvmi, pod, imgPullSecret.SourceSecret, opts); err != nil {
			return err
		}
	}

	return nil
}

// createImporterSettings fills settings for the dvcr-importer binary.
func (r *CVMIReconciler) createImporterSettings(cvmi *virtv2alpha1.ClusterVirtualMachineImage) (*importer.Settings, error) {
	settings := &importer.Settings{
		Verbose: r.verbose,
	}

	ds := cvmi.Spec.DataSource

	switch ds.Type {
	case virtv2alpha1.DataSourceTypeHTTP:
		if ds.HTTP == nil {
			return nil, fmt.Errorf("dataSource '%s' specified without related 'http' section", ds.Type)
		}
		importer.ApplyHTTPSourceSettings(settings, ds.HTTP)
	case virtv2alpha1.DataSourceTypeContainerImage:
		if ds.ContainerImage == nil {
			return nil, fmt.Errorf("dataSource '%s' specified without related 'containerImage' section", ds.Type)
		}
		if annSecret := cvmi.GetAnnotations()[cc.AnnAuthSecret]; annSecret != "" {
			settings.AuthSecret = annSecret
		}
		importer.ApplyRegistrySourceSettings(settings, ds.ContainerImage)
	case virtv2alpha1.DataSourceTypeClusterVirtualMachineImage:
		cvmiRef := ds.ClusterVirtualMachineImage
		if cvmiRef == nil {
			return nil, fmt.Errorf("dataSource '%s' specified without related 'clusterVirtualMachineImage' section", ds.Type)
		}
		dvcrSourceImageName := r.dvcrSettings.RegistryImageForCVMI(cvmiRef.Name)
		importer.ApplyDVCRSourceSettings(settings, dvcrSourceImageName)
	case virtv2alpha1.DataSourceTypeVirtualMachineImage:
		vmiRef := ds.VirtualMachineImage
		if vmiRef == nil {
			return nil, fmt.Errorf("dataSource '%s' specified without related 'virtualMachineImage' section", ds.Type)
		}
		dvcrSourceImageName := r.dvcrSettings.RegistryImageForVMI(vmiRef.Name, vmiRef.Namespace)
		importer.ApplyDVCRSourceSettings(settings, dvcrSourceImageName)
	default:
		return nil, fmt.Errorf("unknown dataSource: %s", ds.Type)
	}

	// Set DVCR destination settings.
	dvcrDestImageName := r.dvcrSettings.RegistryImageForCVMI(cvmi.Name)
	importer.ApplyDVCRDestinationSettings(settings, r.dvcrSettings, dvcrDestImageName)

	// TODO Update proxy settings.

	return settings, nil
}

func (r *CVMIReconciler) createImporterPodSettings(cvmi *virtv2alpha1.ClusterVirtualMachineImage) *importer.PodSettings {
	return &importer.PodSettings{
		Name:            cvmi.Annotations[cc.AnnImportPodName],
		Image:           r.importerImage,
		PullPolicy:      r.pullPolicy,
		Namespace:       r.namespace,
		OwnerReference:  cvmiutil.MakeOwnerReference(cvmi),
		ControllerName:  cvmiControllerName,
		InstallerLabels: r.installerLabels,
	}
}

func (r *CVMIReconciler) createImporterAuthSecret(
	ctx context.Context,
	cvmi *virtv2alpha1.ClusterVirtualMachineImage,
	pod *corev1.Pod,
	srcSecret *corev1.Secret,
	opts two_phase_reconciler.ReconcilerOptions,
) error {
	opts.Log.V(1).Info("Creating importer Secret for Pod", "pod.Name", cvmi.Name)

	importerSecret := importer.NewSecret(r.createImporterAuthSecretSettings(cvmi, pod, srcSecret))

	secret, err := importerSecret.Create(ctx, opts.Client)
	if err != nil {
		return err
	}
	opts.Log.V(1).Info("Created importer Secret", "secret.Name", secret.Name)

	return nil
}

func (r *CVMIReconciler) createImporterAuthSecretSettings(cvmi *virtv2alpha1.ClusterVirtualMachineImage, pod *corev1.Pod, srcSecret *corev1.Secret) *importer.SecretSettings {
	return &importer.SecretSettings{
		Name:           cvmi.Annotations[cc.AnnAuthSecret],
		Namespace:      r.namespace,
		OwnerReference: podutil.MakeOwnerReference(pod),
		Data:           srcSecret.Data,
		Type:           srcSecret.Type,
	}
}
