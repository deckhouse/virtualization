package controller

import (
	"context"
	"errors"
	"fmt"

	virtv2alpha1 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	vmdutil "github.com/deckhouse/virtualization-controller/pkg/common/vmd"
	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
)

func (r *VMDReconciler) startImporterPod(ctx context.Context, vmd *virtv2alpha1.VirtualMachineDisk, opts two_phase_reconciler.ReconcilerOptions) error {
	opts.Log.V(1).Info("Creating importer POD for PVC", "pvc.Name", vmd.Name)

	importerSettings, err := r.createImporterSettings(vmd)
	if err != nil {
		return err
	}

	// all checks passed, let's create the importer pod!
	podSettings := r.createImporterPodSettings(vmd)

	caBundleSettings := importer.NewCABundleSettings(vmdutil.GetCABundle(vmd), vmd.Annotations[cc.AnnCABundleConfigMap])

	imp := importer.NewImporter(podSettings, importerSettings, caBundleSettings)
	pod, err := imp.CreatePod(ctx, opts.Client)
	if err != nil {
		err = cc.PublishPodErr(err, vmd.Annotations[cc.AnnImportPodName], vmd, opts.Recorder, opts.Client)
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
func (r *VMDReconciler) createImporterSettings(vmd *virtv2alpha1.VirtualMachineDisk) (*importer.Settings, error) {
	if vmd.Spec.DataSource == nil {
		return nil, errors.New("no source to create importer settings")
	}

	settings := &importer.Settings{
		Verbose: r.verbose,
	}

	ds := vmd.Spec.DataSource

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
		// Note: use namespace from the current VMD resource.
		dvcrSourceImageName := r.dvcrSettings.RegistryImageForVMI(vmiRef.Name, vmd.Namespace)
		importer.ApplyDVCRSourceSettings(settings, dvcrSourceImageName)
	default:
		return nil, fmt.Errorf("unknown dataSource: %s", ds.Type)
	}

	// Set DVCR destination settings.
	dvcrDestImageName := r.dvcrSettings.RegistryImageForVMD(vmd.Name, vmd.Namespace)
	importer.ApplyDVCRDestinationSettings(settings, r.dvcrSettings, dvcrDestImageName)

	// TODO Update proxy settings.

	return settings, nil
}

func (r *VMDReconciler) createImporterPodSettings(vmd *virtv2alpha1.VirtualMachineDisk) *importer.PodSettings {
	return &importer.PodSettings{
		Name:            vmd.Annotations[cc.AnnImportPodName],
		Image:           r.importerImage,
		PullPolicy:      r.pullPolicy,
		Namespace:       vmd.GetNamespace(),
		OwnerReference:  vmdutil.MakeOwnerReference(vmd),
		ControllerName:  vmdControllerName,
		InstallerLabels: map[string]string{},
	}
}
