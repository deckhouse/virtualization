package controller

import (
	"context"
	"fmt"

	virtv2alpha1 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	vmiutil "github.com/deckhouse/virtualization-controller/pkg/common/vmi"
	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
)

func (r *VMIReconciler) startImporterPod(ctx context.Context, vmi *virtv2alpha1.VirtualMachineImage, opts two_phase_reconciler.ReconcilerOptions) error {
	opts.Log.V(1).Info("Creating importer POD for PVC", "pvc.Name", vmi.Name)

	importerSettings, err := r.createImporterSettings(vmi)
	if err != nil {
		return err
	}

	// all checks passed, let's create the importer pod!
	podSettings := r.createImporterPodSettings(vmi)

	caBundleSettings := importer.NewCABundleSettings(vmiutil.GetCABundle(vmi), vmi.Annotations[cc.AnnCABundleConfigMap])

	imp := importer.NewImporter(podSettings, importerSettings, caBundleSettings)
	pod, err := imp.CreatePod(ctx, opts.Client)
	if err != nil {
		err = cc.PublishPodErr(err, vmi.Annotations[cc.AnnImportPodName], vmi, opts.Recorder, opts.Client)
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

	return nil
}

// createImporterSettings fills settings for the dvcr-importer binary.
func (r *VMIReconciler) createImporterSettings(vmi *virtv2alpha1.VirtualMachineImage) (*importer.Settings, error) {
	settings := &importer.Settings{
		Verbose: r.verbose,
	}

	ds := vmi.Spec.DataSource

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
		// Note: use namespace from the current VMI resource.
		dvcrSourceImageName := r.dvcrSettings.RegistryImageForVMI(vmiRef.Name, vmi.Namespace)
		importer.ApplyDVCRSourceSettings(settings, dvcrSourceImageName)
	default:
		return nil, fmt.Errorf("unknown dataSource: %s", ds.Type)
	}

	// Set DVCR destination settings.
	dvcrDestImageName := r.dvcrSettings.RegistryImageForVMI(vmi.Name, vmi.Namespace)
	importer.ApplyDVCRDestinationSettings(settings, r.dvcrSettings, dvcrDestImageName)

	// TODO Update proxy settings.

	return settings, nil
}

func (r *VMIReconciler) createImporterPodSettings(vmi *virtv2alpha1.VirtualMachineImage) *importer.PodSettings {
	return &importer.PodSettings{
		Name:            vmi.Annotations[cc.AnnImportPodName],
		Image:           r.importerImage,
		PullPolicy:      r.pullPolicy,
		Namespace:       vmi.GetNamespace(),
		OwnerReference:  vmiutil.MakeOwnerReference(vmi),
		ControllerName:  vmiControllerName,
		InstallerLabels: map[string]string{},
	}
}
