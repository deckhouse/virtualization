package controller

import (
	"context"
	"errors"
	"fmt"

	"github.com/deckhouse/virtualization-controller/pkg/common/datasource"
	vmdutil "github.com/deckhouse/virtualization-controller/pkg/common/vmd"
	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
	virtv2alpha1 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func (r *VMDReconciler) startImporterPod(ctx context.Context, state *VMDReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	vmd := state.VMD.Current()
	if vmd.Spec.DataSource == nil {
		opts.Log.Error(errors.New("start importer Pod for empty dataSource"), "Possible bug")
		return nil
	}

	opts.Log.V(1).Info("Creating importer POD for VMD", "vmd.Name", vmd.Name)

	importerSettings, err := r.createImporterSettings(state)
	if err != nil {
		return err
	}

	// all checks passed, let's create the importer pod!
	podSettings := r.createImporterPodSettings(state)

	imp := importer.NewImporter(podSettings, importerSettings)
	pod, err := imp.CreatePod(ctx, opts.Client)
	if err != nil {
		err = cc.PublishPodErr(err, podSettings.Name, vmd, opts.Recorder, opts.Client)
		if err != nil {
			return err
		}
	}

	opts.Log.V(1).Info("Created importer POD", "pod.Name", pod.Name)

	return supplements.EnsureForPod(ctx, opts.Client, state.Supplements, pod, datasource.NewCABundleForVMD(vmd.Spec.DataSource), r.dvcrSettings)
}

// createImporterSettings fills settings for the dvcr-importer binary.
func (r *VMDReconciler) createImporterSettings(state *VMDReconcilerState) (*importer.Settings, error) {
	vmd := state.VMD.Current()

	settings := &importer.Settings{
		Verbose: r.verbose,
	}

	ds := vmd.Spec.DataSource

	switch ds.Type {
	case virtv2alpha1.DataSourceTypeHTTP:
		if ds.HTTP == nil {
			return nil, fmt.Errorf("dataSource '%s' specified without related 'http' section", ds.Type)
		}
		importer.ApplyHTTPSourceSettings(settings, ds.HTTP, state.Supplements)
	case virtv2alpha1.DataSourceTypeContainerImage:
		if ds.ContainerImage == nil {
			return nil, fmt.Errorf("dataSource '%s' specified without related 'containerImage' section", ds.Type)
		}
		importer.ApplyRegistrySourceSettings(settings, ds.ContainerImage, state.Supplements)
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
	importer.ApplyDVCRDestinationSettings(settings, r.dvcrSettings, state.Supplements, dvcrDestImageName)

	// TODO Update proxy settings.

	return settings, nil
}

func (r *VMDReconciler) createImporterPodSettings(state *VMDReconcilerState) *importer.PodSettings {
	importerPod := state.Supplements.ImporterPod()
	return &importer.PodSettings{
		Name:            importerPod.Name,
		Image:           r.importerImage,
		PullPolicy:      r.pullPolicy,
		Namespace:       importerPod.Namespace,
		OwnerReference:  vmdutil.MakeOwnerReference(state.VMD.Current()),
		ControllerName:  vmdControllerName,
		InstallerLabels: map[string]string{},
	}
}
